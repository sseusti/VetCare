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


def make_branches() -> pd.DataFrame:
    rows = []
    for i, (name, address) in enumerate(config.BRANCHES, start=1):
        rows.append({"id": i, "name": name, "address": address})
    return pd.DataFrame(rows)


def make_doctors(rng, fake, branches: pd.DataFrame, used_phones: set[str]) -> pd.DataFrame:
    rows = []
    for i in range(1, config.N_DOCTORS + 1):
        if rng.random() < 0.5:
            last, first, middle = fake.last_name_male(), fake.first_name_male(), fake.middle_name_male()
        else:
            last, first, middle = fake.last_name_female(), fake.first_name_female(), fake.middle_name_female()

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


def main() -> None:
    rng = np.random.default_rng(config.SEED)
    fake = Faker("ru_RU")
    Faker.seed(config.SEED)
    used_phones: set[str] = set()

    OUT.mkdir(parents=True, exist_ok=True)

    branches = make_branches()
    branches.to_csv(OUT / "branches.csv", index=False)
    print(f"branches: {len(branches)}")

    doctors = make_doctors(rng, fake, branches, used_phones)
    doctors.to_csv(OUT / "doctors.csv", index=False)
    print(f"doctors: {len(doctors)}")


if __name__ == "__main__":
    main()
