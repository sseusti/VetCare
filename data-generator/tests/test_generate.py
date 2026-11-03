import re
from datetime import date

import numpy as np
import pandas as pd
import pytest

import config
import generate


@pytest.fixture(scope="module")
def tables() -> dict[str, pd.DataFrame]:
    return generate.build()


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
    a, b = generate.build(), generate.build()
    for name in a:
        pd.testing.assert_frame_equal(a[name], b[name])


def test_different_seed_gives_different_data():
    assert not generate.build(1)["doctors"].equals(generate.build(2)["doctors"])


def test_owners(tables):
    owners = tables["owners"]
    assert len(owners) == config.N_OWNERS
    assert owners["id"].is_unique
    assert all(d <= config.END for d in owners["registered_at"])
    before = (owners["registered_at"] < config.START).mean()
    assert abs(before - config.OWNERS_BEFORE_START) < 0.05
    assert owners["_loyalty"].between(0, 1).all()


def test_phones_unique_across_doctors_and_owners(tables):
    phones = pd.concat([tables["doctors"]["phone"], tables["owners"]["phone"]])
    assert phones.is_unique
    assert phones.str.fullmatch(r"\+79\d{9}").all()


def test_pets_reference_owners_and_every_owner_has_a_pet(tables):
    pets, owners = tables["pets"], tables["owners"]
    assert pets["id"].is_unique
    assert set(pets["owner_id"]) == set(owners["id"])
    assert pets.groupby("owner_id").size().between(1, max(config.PETS_PER_OWNER)).all()


def test_pet_values_match_schema_and_species(tables):
    pets = tables["pets"]
    assert set(pets["sex"]) <= {"M", "F"}
    assert set(pets["species"]) <= set(config.SPECIES)
    for pet in pets.itertuples():
        assert pet.size in config.SIZES[pet.species]
        assert pet.breed in config.BREEDS[pet.species]


def test_pets_born_before_owner_registration(tables):
    pets = tables["pets"].merge(
        tables["owners"][["id", "registered_at"]].rename(columns={"id": "owner_id"}), on="owner_id"
    )
    assert (pets["birth_date"] < pets["registered_at"]).all()


def test_one_medical_card_per_pet(tables):
    cards, pets = tables["medical_cards"], tables["pets"]
    assert cards["pet_id"].is_unique
    assert set(cards["pet_id"]) == set(pets["id"])
    assert cards["created_at"].notna().all()
    share = cards["chronic_diseases"].notna().mean()
    assert abs(share - config.CHRONIC_SHARE) < 0.03


def test_saved_csv_has_no_internal_columns(tmp_path, monkeypatch, tables):
    monkeypatch.setattr(generate, "OUT", tmp_path)
    generate.save(tables["owners"], "owners")
    columns = pd.read_csv(tmp_path / "owners.csv", nrows=0).columns
    assert not any(c.startswith("_") for c in columns)
