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


@pytest.fixture
def small(monkeypatch):
    """Уменьшенная клиника: полный build занимает секунды, для проверок seed хватит 300 владельцев."""
    monkeypatch.setattr(config, "N_OWNERS", 300)


def test_same_seed_gives_same_data(small):
    a, b = generate.build(), generate.build()
    for name in a:
        pd.testing.assert_frame_equal(a[name], b[name])


def test_different_seed_gives_different_data(small):
    assert not generate.build(1)["doctors"].equals(generate.build(2)["doctors"])


# ---- владельцы, животные, медкарты ----


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


# ---- приёмы ----

STATUSES = {"scheduled", "completed", "cancelled", "no_show"}


@pytest.fixture(scope="module")
def visits(tables) -> pd.DataFrame:
    v = tables["visits"].copy()
    v["day"] = v["scheduled_at"].dt.date
    return v


def test_visits_volume_and_values(visits):
    assert 20_000 <= len(visits) <= 35_000
    assert list(visits["id"]) == list(range(1, len(visits) + 1))
    assert set(visits["status"]) <= STATUSES
    assert set(visits["visit_type"]) <= set(config.VISIT_TYPES)
    assert visits["status"].eq("completed").mean() > 0.75


def test_visits_reference_cards_and_doctors(tables, visits):
    assert set(visits["medical_card_id"]) <= set(tables["medical_cards"]["id"])
    doctors = tables["doctors"].set_index("id")
    assert set(visits["doctor_id"]) <= set(doctors.index)
    # Приём проходит в филиале своего врача и только после его найма.
    assert (visits["branch_id"] == visits["doctor_id"].map(doctors["branch_id"])).all()
    assert (visits["day"] >= visits["doctor_id"].map(doctors["hired_at"])).all()


def test_surgery_only_by_surgeons(tables, visits):
    spec = tables["doctors"].set_index("id")["specialization"]
    surgery = visits[visits["visit_type"] == "surgery"]
    assert (surgery["doctor_id"].map(spec) == config.SURGEON).all()
    assert (surgery["cabinet"] == "операционная").all()


def test_visit_dates(tables, visits):
    assert (visits["booked_at"] <= visits["scheduled_at"]).all()
    registered = visits["_owner_id"].map(tables["owners"].set_index("id")["registered_at"])
    assert (visits["day"] >= registered).all()
    assert (visits["day"] >= config.START).all()
    # В будущем — только запланированные приёмы, в прошлом — ни одного запланированного.
    future = visits["day"] > config.END
    assert (visits.loc[future, "status"] == "scheduled").all()
    assert (visits.loc[~future, "status"] != "scheduled").all()


def test_diagnosis_only_for_completed(visits):
    completed = visits["status"] == "completed"
    assert visits.loc[completed, "diagnosis"].notna().all()
    assert visits.loc[~completed, "diagnosis"].isna().all()
    assert visits.loc[~completed, "recommendations"].isna().all()


def test_no_vaccination_for_species_without_vaccines(visits):
    vaccinated = set(visits.loc[visits["visit_type"] == "vaccination", "_species"])
    assert all(config.VACCINES[s] for s in vaccinated)


def test_reminders_only_in_experiment(visits):
    with_reminder = visits[visits["reminder_sent"]]
    assert (with_reminder["day"] >= config.EXPERIMENT_START).all()
    assert (with_reminder["_lead_days"] > 0).all()
    eligible = visits[(visits["day"] >= config.EXPERIMENT_START) & (visits["_lead_days"] > 0)]
    assert abs(eligible["reminder_sent"].mean() - config.REMINDER_SHARE) < 0.03


# ---- заложенные закономерности видны в данных ----


@pytest.fixture(scope="module")
def attended(visits) -> pd.DataFrame:
    """Приёмы, на которые клиент мог прийти или не прийти."""
    d = visits[visits["status"].isin(["completed", "no_show"])].copy()
    d["no_show"] = d["status"] == "no_show"
    return d


def test_reminder_reduces_no_show(attended):
    eligible = attended[(attended["day"] >= config.EXPERIMENT_START) & (attended["_lead_days"] > 0)]
    rate = eligible.groupby("reminder_sent")["no_show"].mean()
    assert rate[True] < rate[False] * 0.9


def test_monday_has_more_no_show(attended):
    monday = attended["scheduled_at"].dt.weekday == 0
    assert attended.loc[monday, "no_show"].mean() > attended.loc[~monday, "no_show"].mean() * 1.2


def test_spring_and_summer_busier_than_winter(visits):
    by_month = visits[visits["day"] <= config.END]["scheduled_at"].dt.month.value_counts()
    assert by_month[[5, 6, 7]].mean() > by_month[[1, 2, 12]].mean() * 1.3


# ---- оплаты и прививки ----


def test_one_payment_per_completed_visit(tables, visits):
    payments = tables["payments"]
    completed = visits.loc[visits["status"] == "completed", "id"]
    assert payments["visit_id"].is_unique
    assert set(payments["visit_id"]) == set(completed)


def test_payment_values(tables, visits):
    payments = tables["payments"]
    assert (payments["amount"] > 0).all()
    assert (payments["amount"] % 50 == 0).all()
    assert payments["discount"].between(0, 100).all()
    assert set(payments["payment_method"]) <= set(config.PAYMENT_METHODS)
    start = payments["visit_id"].map(visits.set_index("id")["scheduled_at"])
    assert (payments["paid_at"] > start).all()


def test_surgery_is_the_most_expensive(tables):
    by_service = tables["payments"].groupby("service")["amount"].mean()
    assert by_service.idxmax() == "surgery"


def test_vaccinations_come_from_completed_vaccination_visits(tables, visits):
    vaccinations = tables["vaccinations"]
    done = visits[(visits["status"] == "completed") & (visits["visit_type"] == "vaccination")]
    assert len(vaccinations) == len(done)
    assert set(vaccinations["pet_id"]) <= set(tables["pets"]["id"])
    species = vaccinations["pet_id"].map(tables["pets"].set_index("id")["species"])
    assert all(v in config.VACCINES[s] for v, s in zip(vaccinations["vaccine"], species, strict=True))
