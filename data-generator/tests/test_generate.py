import re
from datetime import date

import numpy as np
import pandas as pd
import pytest
from faker import Faker

import config
import generate


def build(seed: int = config.SEED) -> dict[str, pd.DataFrame]:
    """Собирает таблицы так же, как main(), но без записи на диск."""
    rng = np.random.default_rng(seed)
    fake = Faker("ru_RU")
    Faker.seed(seed)
    used_phones: set[str] = set()
    branches = generate.make_branches()
    doctors = generate.make_doctors(rng, fake, branches, used_phones)
    return {"branches": branches, "doctors": doctors}


@pytest.fixture(scope="module")
def tables() -> dict[str, pd.DataFrame]:
    return build()


def test_make_phone_format_and_uniqueness():
    rng = np.random.default_rng(0)
    used: set[str] = set()
    phones = [generate.make_phone(rng, used) for _ in range(1000)]
    assert len(set(phones)) == 1000
    assert used == set(phones)
    assert all(re.fullmatch(r"\+79\d{9}", p) for p in phones)


def test_branches(tables):
    branches = tables["branches"]
    assert list(branches["id"]) == list(range(1, len(config.BRANCHES) + 1))
    assert branches["name"].is_unique


def test_doctors_reference_existing_branches(tables):
    doctors, branches = tables["doctors"], tables["branches"]
    assert len(doctors) == config.N_DOCTORS
    assert doctors["id"].is_unique
    assert set(doctors["branch_id"]) <= set(branches["id"])
    # В каждом филиале есть хотя бы один врач.
    assert set(doctors["branch_id"]) == set(branches["id"])


def test_doctors_phones_unique_and_normalized(tables):
    phones = tables["doctors"]["phone"]
    assert phones.is_unique
    assert phones.str.fullmatch(r"\+79\d{9}").all()


def test_doctors_values_match_schema(tables):
    doctors = tables["doctors"]
    assert set(doctors["specialization"]) <= set(config.SPECIALIZATIONS)
    assert doctors[["last_name", "first_name", "phone"]].notna().all().all()


def test_new_doctors_hired_inside_period(tables):
    hired = tables["doctors"].sort_values("id")["hired_at"]
    n_old = config.N_DOCTORS - config.N_NEW_DOCTORS
    old, new = hired.iloc[:n_old], hired.iloc[n_old:]
    assert all(d < config.START for d in old)
    assert all(config.START < d <= config.END for d in new)
    assert all(isinstance(d, date) for d in hired)


def test_same_seed_gives_same_data():
    a, b = build(), build()
    for name in a:
        pd.testing.assert_frame_equal(a[name], b[name])


def test_different_seed_gives_different_data():
    assert not build(1)["doctors"].equals(build(2)["doctors"])
