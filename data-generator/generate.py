from datetime import timedelta
from pathlib import Path

import numpy as np
import pandas as pd
from faker import Faker

import config

OUT = Path(__file__).parent / "data" / "raw"


def make_phone(rng, used: set[str]) -> str:
    while True:
        phone = "+79" + "".join(str(d) for d in rng.integers(0, 10, size=9))
        if phone not in used:
            used.add(phone)
            return phone


def random_person(rng, fake) -> tuple[str, str, str]:
    """Фамилия, имя и отчество одного пола."""
    if rng.random() < 0.5:
        return fake.last_name_male(), fake.first_name_male(), fake.middle_name_male()
    return fake.last_name_female(), fake.first_name_female(), fake.middle_name_female()


def save(df: pd.DataFrame, name: str) -> None:
    """Пишет CSV без служебных колонок: те, что начинаются с «_», нужны только генератору."""
    public = [c for c in df.columns if not c.startswith("_")]
    df[public].to_csv(OUT / f"{name}.csv", index=False)


def make_branches() -> pd.DataFrame:
    rows = []
    for i, (name, address) in enumerate(config.BRANCHES, start=1):
        rows.append({"id": i, "name": name, "address": address})
    return pd.DataFrame(rows)


def make_doctors(rng, fake, branches: pd.DataFrame, used_phones: set[str]) -> pd.DataFrame:
    rows = []
    for i in range(1, config.N_DOCTORS + 1):
        last, first, middle = random_person(rng, fake)

        is_new = i > config.N_DOCTORS - config.N_NEW_DOCTORS
        if is_new:
            days_after = int(rng.integers(*config.NEW_DOCTOR_DAYS))
            hired_at = config.START + timedelta(days=days_after)
        else:
            days_before = int(rng.integers(0, 3 * 365))
            hired_at = config.START - timedelta(days=days_before)

        rows.append(
            {
                "id": i,
                "last_name": last,
                "first_name": first,
                "middle_name": middle,
                "phone": make_phone(rng, used_phones),
                "specialization": rng.choice(config.SPECIALIZATIONS, p=config.SPECIALIZATION_WEIGHTS),
                "branch_id": (i - 1) % len(branches) + 1,
                "hired_at": hired_at,
            }
        )
    return pd.DataFrame(rows)


def make_owners(rng, fake, used_phones: set[str]) -> pd.DataFrame:
    period_days = (config.END - config.START).days
    rows = []
    for i in range(1, config.N_OWNERS + 1):
        last, first, middle = random_person(rng, fake)
        if rng.random() < config.OWNERS_BEFORE_START:
            registered_at = config.START - timedelta(days=int(rng.integers(1, config.OWNER_HISTORY_DAYS)))
        else:
            registered_at = config.START + timedelta(days=int(rng.integers(0, period_days + 1)))
        rows.append(
            {
                "id": i,
                "last_name": last,
                "first_name": first,
                "middle_name": middle,
                "phone": make_phone(rng, used_phones),
                "registered_at": registered_at,
                # Насколько часто владелец ходит в клинику; используется при генерации приёмов.
                "_loyalty": float(rng.beta(*config.LOYALTY_BETA)),
            }
        )
    return pd.DataFrame(rows)


def make_pets(rng, owners: pd.DataFrame) -> pd.DataFrame:
    rows = []
    pet_id = 1
    for owner in owners.itertuples():
        n_pets = int(rng.choice(config.PETS_PER_OWNER, p=config.PETS_PER_OWNER_WEIGHTS))
        for _ in range(n_pets):
            species = str(rng.choice(config.SPECIES, p=config.SPECIES_WEIGHTS))
            age_days = int(rng.integers(*config.PET_AGE_DAYS))
            rows.append(
                {
                    "id": pet_id,
                    "owner_id": owner.id,
                    "name": str(rng.choice(config.PET_NAMES)),
                    "species": species,
                    "breed": str(rng.choice(config.BREEDS[species])),
                    "color": str(rng.choice(config.COLORS)),
                    "sex": str(rng.choice(["M", "F"])),
                    "size": str(rng.choice(config.SIZES[species])),
                    "birth_date": owner.registered_at - timedelta(days=age_days),
                }
            )
            pet_id += 1
    return pd.DataFrame(rows)


def make_medical_cards(rng, pets: pd.DataFrame, owners: pd.DataFrame) -> pd.DataFrame:
    # JOIN животных с владельцами: карта заводится в день регистрации владельца.
    registered = pets.merge(
        owners[["id", "registered_at"]].rename(columns={"id": "owner_id"}),
        on="owner_id",
        how="left",
    )
    rows = []
    for card_id, pet in enumerate(registered.itertuples(), start=1):
        chronic = str(rng.choice(config.CHRONIC_DISEASES)) if rng.random() < config.CHRONIC_SHARE else None
        allergy = str(rng.choice(config.ALLERGIES)) if rng.random() < config.ALLERGY_SHARE else None
        rows.append(
            {
                "id": card_id,
                "pet_id": pet.id,
                "chronic_diseases": chronic,
                "allergies": allergy,
                "created_at": pet.registered_at,
            }
        )
    return pd.DataFrame(rows)


def build(seed: int = config.SEED) -> dict[str, pd.DataFrame]:
    """Генерирует все таблицы в порядке зависимостей. Одинаковый seed — одинаковые данные."""
    rng = np.random.default_rng(seed)
    fake = Faker("ru_RU")
    Faker.seed(seed)
    used_phones: set[str] = set()  # общее для врачей и владельцев

    branches = make_branches()
    doctors = make_doctors(rng, fake, branches, used_phones)
    owners = make_owners(rng, fake, used_phones)
    pets = make_pets(rng, owners)
    medical_cards = make_medical_cards(rng, pets, owners)
    return {
        "branches": branches,
        "doctors": doctors,
        "owners": owners,
        "pets": pets,
        "medical_cards": medical_cards,
    }


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for name, df in build().items():
        save(df, name)
        print(f"{name}: {len(df)}")


if __name__ == "__main__":
    main()
