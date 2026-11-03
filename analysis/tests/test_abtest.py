import numpy as np
import pandas as pd
import pytest
from scipy import stats

import abtest

# ---- SRM и баланс ----


def test_srm_passes_on_even_split():
    result = abtest.srm_test(5000, 5030)
    assert result.ok
    assert result.share_b == pytest.approx(5030 / 10030)


def test_srm_fails_on_broken_split():
    assert not abtest.srm_test(5000, 5400).ok


def test_srm_respects_expected_share():
    assert abtest.srm_test(9000, 1000, expected_share_b=0.1).ok
    assert not abtest.srm_test(9000, 1000).ok


def test_smd_zero_for_same_distribution():
    x = np.arange(100)
    assert abtest.standardized_mean_difference(x, x) == pytest.approx(0)


def test_smd_one_for_shift_by_sd():
    rng = np.random.default_rng(1)
    a = rng.normal(0, 1, 100_000)
    b = rng.normal(1, 1, 100_000)
    assert abtest.standardized_mean_difference(a, b) == pytest.approx(1, abs=0.02)


def test_smd_constant_feature():
    assert abtest.standardized_mean_difference(np.ones(5), np.ones(5)) == 0


# ---- Доли ----


def test_wilson_ci_contains_estimate_and_stays_in_bounds():
    low, high = abtest.wilson_ci(0, 20)
    assert low == pytest.approx(0, abs=1e-12) and 0 < high < 0.2
    low, high = abtest.wilson_ci(9, 100)
    assert low < 0.09 < high


def test_wilson_ci_empty_sample():
    assert abtest.wilson_ci(0, 0) == (0.0, 1.0)


def test_ztest_matches_chi_square():
    # z-тест для двух долей эквивалентен хи-квадрату для таблицы 2×2 без поправки Йейтса: z² = χ².
    x_a, n_a, x_b, n_b = 408, 4470, 314, 4546
    result = abtest.two_proportion_ztest(x_a, n_a, x_b, n_b)
    table = [[x_a, n_a - x_a], [x_b, n_b - x_b]]
    chi2, p_value, _, _ = stats.chi2_contingency(table, correction=False)
    assert result.z**2 == pytest.approx(chi2)
    assert result.p_value == pytest.approx(p_value)


def test_ztest_effect_and_intervals():
    result = abtest.two_proportion_ztest(100, 1000, 70, 1000)
    assert result.diff == pytest.approx(-0.03)
    assert result.lift == pytest.approx(-0.3)
    assert result.diff_ci[0] < result.diff < result.diff_ci[1]
    assert result.lift_ci[0] < result.lift < result.lift_ci[1]
    assert result.significant


def test_ztest_no_effect_not_significant():
    result = abtest.two_proportion_ztest(90, 1000, 91, 1000)
    assert not result.significant
    assert result.diff_ci[0] < 0 < result.diff_ci[1]


def test_ztest_interval_coverage():
    # 95%-интервал должен накрывать истинную разницу примерно в 95% повторений.
    rng = np.random.default_rng(7)
    p_a, p_b, n = 0.09, 0.065, 4000
    covered = 0
    for _ in range(400):
        r = abtest.two_proportion_ztest(rng.binomial(n, p_a), n, rng.binomial(n, p_b), n)
        covered += r.diff_ci[0] <= p_b - p_a <= r.diff_ci[1]
    assert 0.92 <= covered / 400 <= 0.98


# ---- Мощность ----


def test_sample_size_and_power_agree():
    n = abtest.sample_size(0.09, -0.3)
    assert abtest.power(0.09, -0.3, n) == pytest.approx(0.8, abs=0.01)


def test_mde_shrinks_with_sample():
    assert abtest.mde(0.09, 16_000) == pytest.approx(abtest.mde(0.09, 4000) / 2)


def test_mde_consistent_with_sample_size():
    n = 4000
    effect = abtest.mde(0.09, n)
    # MDE считает дисперсию обеих групп по базовой доле; при снижении доли дисперсия теста меньше,
    # поэтому реальная мощность на MDE не ниже 80% — оценка консервативная
    assert 0.8 <= abtest.power(0.09, -effect / 0.09, n) <= 0.87


def test_smaller_effect_needs_bigger_sample():
    assert abtest.sample_size(0.09, -0.1) > 5 * abtest.sample_size(0.09, -0.3)


# ---- Бутстрап ----


def make_clustered(rng, n_owners=800, effect=0.0):
    rows = []
    for owner in range(n_owners):
        base = rng.beta(2, 20)  # у каждого клиента своя склонность не приходить
        for _ in range(rng.integers(1, 6)):
            group = "treatment" if rng.random() < 0.5 else "control"
            p = base * (1 + effect) if group == "treatment" else base
            rows.append((owner, group, rng.random() < p))
    return pd.DataFrame(rows, columns=["owner_id", "ab_group", "is_no_show"])


def test_cluster_bootstrap_point_matches_simple_diff():
    df = make_clustered(np.random.default_rng(3), effect=-0.3)
    result = abtest.cluster_bootstrap_diff(df, "owner_id", "ab_group", "is_no_show", n_boot=500)
    rates = df.groupby("ab_group")["is_no_show"].mean()
    assert result["diff"] == pytest.approx(rates["treatment"] - rates["control"])
    assert result["ci"][0] < result["diff"] < result["ci"][1]


def test_cluster_bootstrap_is_reproducible():
    df = make_clustered(np.random.default_rng(4))
    a = abtest.cluster_bootstrap_diff(df, "owner_id", "ab_group", "is_no_show", n_boot=200, seed=1)
    b = abtest.cluster_bootstrap_diff(df, "owner_id", "ab_group", "is_no_show", n_boot=200, seed=1)
    assert a == b


# ---- CUPED ----


def test_cuped_keeps_effect_and_reduces_variance():
    rng = np.random.default_rng(5)
    n = 20_000
    x = rng.normal(0, 1, n)
    in_b = rng.random(n) < 0.5
    y = 2 * x + rng.normal(0, 1, n) + 0.1 * in_b

    adjusted, theta = abtest.cuped(y, x)
    assert theta == pytest.approx(2, abs=0.05)
    assert adjusted.var() < y.var() / 3

    raw = abtest.welch_ttest(y[~in_b], y[in_b])
    cup = abtest.welch_ttest(adjusted[~in_b], adjusted[in_b])
    # обе оценки несмещённые — истинный эффект 0.1 в пределах трёх стандартных ошибок,
    # но у CUPED ошибка более чем вдвое меньше
    assert abs(raw.diff - 0.1) < 3 * raw.se
    assert abs(cup.diff - 0.1) < 3 * cup.se
    assert cup.se < raw.se / 2


def test_welch_matches_scipy():
    rng = np.random.default_rng(6)
    a, b = rng.normal(0, 1, 300), rng.normal(0.3, 2, 500)
    result = abtest.welch_ttest(a, b)
    assert result.p_value == pytest.approx(stats.ttest_ind(b, a, equal_var=False).pvalue)
    assert result.ci[0] < result.diff < result.ci[1]


# ---- A/A и подглядывание ----


def test_aa_false_positive_rate_near_alpha():
    y = np.random.default_rng(8).random(5000) < 0.09
    pvalues = abtest.aa_pvalues(y, n_sims=600, seed=2)
    assert 0.025 <= (pvalues < 0.05).mean() <= 0.08


def test_peeking_inflates_false_positives():
    rng = np.random.default_rng(9)
    days = np.repeat(np.arange(60), 100)
    y = rng.random(len(days)) < 0.09
    result = abtest.peeking_false_positive_rate(y, days, n_sims=300, seed=3)
    assert result["looks"] == 60
    assert result["fpr_final"] < 0.1
    assert result["fpr_peeking"] > 2 * result["fpr_final"]


# ---- Множественные сравнения ----


def test_holm_adjust():
    adjusted = abtest.holm_adjust([0.01, 0.04, 0.03, 0.5])
    np.testing.assert_allclose(adjusted, [0.04, 0.09, 0.09, 0.5])


def test_holm_caps_at_one():
    assert abtest.holm_adjust([0.6, 0.7]).max() == 1.0
