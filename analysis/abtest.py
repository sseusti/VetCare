"""Статистика для A/B-тестов: проверка сплита, тест долей, мощность, бутстрап, CUPED, A/A и подглядывание.

Каждая функция — одна формула или одна процедура, которую можно объяснить на собеседовании.
Обозначения: группа A — контроль, группа B — тест; разница всегда «B минус A».
"""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np
import pandas as pd
from scipy import stats

# ---------------------------------------------------------------------------
# Проверка сплита
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class SRMResult:
    n_a: int
    n_b: int
    share_b: float
    chi2: float
    p_value: float

    @property
    def ok(self) -> bool:
        # Для SRM берут строгий порог: ложная тревога дешевле, чем анализ сломанного сплита.
        return self.p_value >= 0.001


def srm_test(n_a: int, n_b: int, expected_share_b: float = 0.5) -> SRMResult:
    """Sample Ratio Mismatch: совпадает ли фактическое разбиение с задуманным.

    Хи-квадрат сравнивает наблюдаемые размеры групп с ожидаемыми. Маленькое p-value значит,
    что сплит сломан (баг в рандомизации, потеря логов) и результатам теста верить нельзя.
    """
    total = n_a + n_b
    expected = [total * (1 - expected_share_b), total * expected_share_b]
    chi2, p_value = stats.chisquare([n_a, n_b], f_exp=expected)
    return SRMResult(n_a, n_b, n_b / total, float(chi2), float(p_value))


def standardized_mean_difference(a: np.ndarray, b: np.ndarray) -> float:
    """Стандартизованная разница средних (SMD) — мера баланса признака между группами.

    Разница средних в единицах общего стандартного отклонения. |SMD| < 0.1 считается хорошим
    балансом: группы похожи по этому признаку, рандомизация сработала.
    """
    a, b = np.asarray(a, float), np.asarray(b, float)
    pooled_sd = np.sqrt((a.var(ddof=1) + b.var(ddof=1)) / 2)
    if pooled_sd == 0:
        return 0.0
    return float((b.mean() - a.mean()) / pooled_sd)


# ---------------------------------------------------------------------------
# Сравнение долей
# ---------------------------------------------------------------------------


def wilson_ci(successes: int, n: int, alpha: float = 0.05) -> tuple[float, float]:
    """Доверительный интервал Уилсона для доли.

    Точнее обычного «p ± 1.96·se» при маленьких долях и выборках и не выходит за [0, 1].
    """
    if n == 0:
        return (0.0, 1.0)
    z = stats.norm.ppf(1 - alpha / 2)
    p = successes / n
    denom = 1 + z**2 / n
    center = (p + z**2 / (2 * n)) / denom
    half = z * np.sqrt(p * (1 - p) / n + z**2 / (4 * n**2)) / denom
    return (float(center - half), float(center + half))


@dataclass(frozen=True)
class ProportionTestResult:
    p_a: float
    p_b: float
    diff: float  # абсолютная разница, B − A
    diff_ci: tuple[float, float]
    lift: float  # относительное изменение, B / A − 1
    lift_ci: tuple[float, float]
    z: float
    p_value: float

    @property
    def significant(self) -> bool:
        return self.p_value < 0.05


def two_proportion_ztest(x_a: int, n_a: int, x_b: int, n_b: int, alpha: float = 0.05) -> ProportionTestResult:
    """Z-тест для разницы двух долей.

    p-value считается с объединённой долей: при нулевой гипотезе доли равны, поэтому разброс
    оцениваем по обеим группам вместе. Интервал для разницы — без объединения (у каждой группы
    свой разброс). Интервал для относительного эффекта — дельта-методом на логарифме отношения:
    log(p_b / p_a) распределён почти нормально, его дисперсия ≈ (1−p_a)/x_a + (1−p_b)/x_b.
    """
    p_a, p_b = x_a / n_a, x_b / n_b
    z_crit = stats.norm.ppf(1 - alpha / 2)

    pooled = (x_a + x_b) / (n_a + n_b)
    se_pooled = np.sqrt(pooled * (1 - pooled) * (1 / n_a + 1 / n_b))
    z = (p_b - p_a) / se_pooled
    p_value = 2 * stats.norm.sf(abs(z))

    se = np.sqrt(p_a * (1 - p_a) / n_a + p_b * (1 - p_b) / n_b)
    diff = p_b - p_a
    diff_ci = (diff - z_crit * se, diff + z_crit * se)

    log_ratio = np.log(p_b / p_a)
    se_log = np.sqrt((1 - p_a) / x_a + (1 - p_b) / x_b)
    lift_ci = (np.exp(log_ratio - z_crit * se_log) - 1, np.exp(log_ratio + z_crit * se_log) - 1)

    return ProportionTestResult(
        p_a=float(p_a),
        p_b=float(p_b),
        diff=float(diff),
        diff_ci=(float(diff_ci[0]), float(diff_ci[1])),
        lift=float(p_b / p_a - 1),
        lift_ci=(float(lift_ci[0]), float(lift_ci[1])),
        z=float(z),
        p_value=float(p_value),
    )


# ---------------------------------------------------------------------------
# Мощность
# ---------------------------------------------------------------------------


def mde(p_base: float, n_per_group: int, alpha: float = 0.05, power: float = 0.8) -> float:
    """Минимальный детектируемый эффект (MDE) — абсолютная разница долей.

    Самый маленький эффект, который тест с такой выборкой найдёт с вероятностью `power`
    при уровне значимости `alpha`. Если ожидаемый эффект меньше MDE, тест запускать бессмысленно:
    скорее всего он ничего не покажет, даже если эффект есть.
    """
    z_alpha = stats.norm.ppf(1 - alpha / 2)
    z_power = stats.norm.ppf(power)
    return float((z_alpha + z_power) * np.sqrt(2 * p_base * (1 - p_base) / n_per_group))


def sample_size(p_base: float, relative_effect: float, alpha: float = 0.05, power: float = 0.8) -> int:
    """Размер каждой группы, чтобы заметить относительный эффект `relative_effect`.

    Пример: p_base = 0.09, relative_effect = −0.3 — хотим заметить снижение неявок с 9% до 6.3%.
    """
    p_new = p_base * (1 + relative_effect)
    z_alpha = stats.norm.ppf(1 - alpha / 2)
    z_power = stats.norm.ppf(power)
    variance = p_base * (1 - p_base) + p_new * (1 - p_new)
    return int(np.ceil((z_alpha + z_power) ** 2 * variance / (p_new - p_base) ** 2))


def power(p_base: float, relative_effect: float, n_per_group: int, alpha: float = 0.05) -> float:
    """Мощность — вероятность заметить эффект `relative_effect` при выборке `n_per_group`."""
    p_new = p_base * (1 + relative_effect)
    se = np.sqrt((p_base * (1 - p_base) + p_new * (1 - p_new)) / n_per_group)
    z_alpha = stats.norm.ppf(1 - alpha / 2)
    return float(stats.norm.sf(z_alpha - abs(p_new - p_base) / se))


# ---------------------------------------------------------------------------
# Кластерный бутстрап
# ---------------------------------------------------------------------------


def cluster_bootstrap_diff(
    df: pd.DataFrame,
    cluster: str,
    group: str,
    outcome: str,
    treatment_label: str = "treatment",
    n_boot: int = 2000,
    alpha: float = 0.05,
    seed: int = 0,
) -> dict:
    """Бутстрап разницы долей с пересэмплированием кластеров (клиентов), а не строк.

    Рандомизировали приёмы, но у одного клиента их несколько, и его приёмы похожи между собой —
    наблюдения не независимы, и обычная формула может занижать разброс. Бутстрап по клиентам
    это учитывает: в каждой итерации клиент попадает в выборку целиком, со всеми приёмами.

    Используется пуассоновский бутстрап: каждый клиент получает вес ~ Poisson(1) — это почти то же,
    что выборка с возвращением, но считается одной матричной операцией.
    """
    is_b = df[group] == treatment_label
    agg = (
        df.assign(_b=is_b, _y=df[outcome].astype(float))
        .groupby([cluster, "_b"])["_y"]
        .agg(["sum", "count"])
        .unstack("_b", fill_value=0)
    )
    x_a, n_a = agg[("sum", False)].to_numpy(), agg[("count", False)].to_numpy()
    x_b, n_b = agg[("sum", True)].to_numpy(), agg[("count", True)].to_numpy()

    rng = np.random.default_rng(seed)
    weights = rng.poisson(1.0, size=(n_boot, len(agg)))
    diffs = (weights @ x_b) / (weights @ n_b) - (weights @ x_a) / (weights @ n_a)

    point = x_b.sum() / n_b.sum() - x_a.sum() / n_a.sum()
    low, high = np.quantile(diffs, [alpha / 2, 1 - alpha / 2])
    return {"diff": float(point), "ci": (float(low), float(high)), "se": float(diffs.std(ddof=1))}


# ---------------------------------------------------------------------------
# CUPED
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class MeanTestResult:
    diff: float
    ci: tuple[float, float]
    se: float
    p_value: float


def welch_ttest(y_a: np.ndarray, y_b: np.ndarray, alpha: float = 0.05) -> MeanTestResult:
    """T-тест Уэлча для разницы средних (без допущения о равных дисперсиях)."""
    y_a, y_b = np.asarray(y_a, float), np.asarray(y_b, float)
    se = np.sqrt(y_a.var(ddof=1) / len(y_a) + y_b.var(ddof=1) / len(y_b))
    diff = y_b.mean() - y_a.mean()
    p_value = stats.ttest_ind(y_b, y_a, equal_var=False).pvalue
    z = stats.norm.ppf(1 - alpha / 2)
    ci = (float(diff - z * se), float(diff + z * se))
    return MeanTestResult(float(diff), ci, float(se), float(p_value))


def cuped(y: np.ndarray, x: np.ndarray) -> tuple[np.ndarray, float]:
    """CUPED: уменьшает разброс метрики с помощью ковариаты, известной до эксперимента.

    y_cuped = y − θ·(x − mean(x)), где θ = cov(x, y) / var(x).
    Ковариата x не должна зависеть от группы (её знали до рандомизации), поэтому среднее y_cuped
    в каждой группе сдвигается одинаково и разница групп не меняется, а дисперсия падает
    в (1 − corr(x, y)²) раз. Меньше дисперсия — уже интервал, меньше нужна выборка.
    """
    y, x = np.asarray(y, float), np.asarray(x, float)
    theta = np.cov(x, y, ddof=1)[0, 1] / x.var(ddof=1)
    return y - theta * (x - x.mean()), float(theta)


# ---------------------------------------------------------------------------
# A/A-тесты и подглядывание
# ---------------------------------------------------------------------------


def aa_pvalues(outcome: np.ndarray, n_sims: int = 1000, seed: int = 0) -> np.ndarray:
    """A/A-тесты: много раз случайно делим одни и те же данные пополам и считаем p-value.

    Настоящей разницы нет, поэтому p-value должны быть равномерны на [0, 1], а доля p < 0.05 —
    около 5%. Если доля заметно больше, критерий врёт на этих данных и ему нельзя доверять.
    """
    y = np.asarray(outcome, float)
    rng = np.random.default_rng(seed)
    n = len(y)
    pvalues = np.empty(n_sims)
    for i in range(n_sims):
        in_b = rng.random(n) < 0.5
        x_b, n_b = y[in_b].sum(), in_b.sum()
        x_a, n_a = y[~in_b].sum(), n - n_b
        pvalues[i] = two_proportion_ztest(int(x_a), int(n_a), int(x_b), int(n_b)).p_value
    return pvalues


def peeking_false_positive_rate(
    outcome: np.ndarray, day: np.ndarray, n_sims: int = 500, alpha: float = 0.05, seed: int = 0
) -> dict:
    """Насколько растёт доля ложных срабатываний, если смотреть на результат каждый день.

    A/A-тест: эффекта нет. «Честный» аналитик смотрит один раз в конце — ошибается в ~alpha
    случаев. «Подглядывающий» проверяет p-value после каждого дня и останавливает тест при первом
    p < alpha — и ошибается гораздо чаще. Поэтому длительность теста фиксируют заранее.
    """
    y = np.asarray(outcome, float)
    days = np.asarray(day)
    order = np.argsort(days, kind="stable")
    y, days = y[order], days[order]
    _, day_end = np.unique(days, return_index=True)
    day_end = np.append(day_end[1:], len(y)) - 1  # индекс последнего наблюдения каждого дня

    rng = np.random.default_rng(seed)
    z_crit = stats.norm.ppf(1 - alpha / 2)
    final_hits = peek_hits = 0
    for _ in range(n_sims):
        in_b = rng.random(len(y)) < 0.5
        x_b = np.cumsum(y * in_b)[day_end]
        n_b = np.cumsum(in_b)[day_end]
        x_a = np.cumsum(y * ~in_b)[day_end]
        n_a = np.cumsum(~in_b)[day_end]
        with np.errstate(divide="ignore", invalid="ignore"):
            pooled = (x_a + x_b) / (n_a + n_b)
            se = np.sqrt(pooled * (1 - pooled) * (1 / n_a + 1 / n_b))
            z = np.abs(x_b / n_b - x_a / n_a) / se
        z = np.nan_to_num(z)
        final_hits += z[-1] > z_crit
        peek_hits += (z > z_crit).any()
    return {"fpr_final": final_hits / n_sims, "fpr_peeking": peek_hits / n_sims, "looks": len(day_end)}


# ---------------------------------------------------------------------------
# Множественные сравнения
# ---------------------------------------------------------------------------


def holm_adjust(pvalues: list[float] | np.ndarray) -> np.ndarray:
    """Поправка Холма на множественные сравнения.

    Когда проверяем эффект в нескольких сегментах, шанс случайно найти «значимый» растёт с каждым
    сегментом. Холм сортирует p-value и умножает k-е по возрастанию на (m − k + 1); результат
    сравнивают с обычным порогом 0.05. Строже ничего не делать, мягче Бонферрони.
    """
    p = np.asarray(pvalues, float)
    m = len(p)
    order = np.argsort(p)
    adjusted = np.empty(m)
    running_max = 0.0
    for rank, idx in enumerate(order):
        running_max = max(running_max, (m - rank) * p[idx])
        adjusted[idx] = min(running_max, 1.0)
    return adjusted
