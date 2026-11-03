from datetime import date, datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

import numpy as np
import pandas as pd
from faker import Faker

import config

OUT = Path(__file__).parent / "data" / "raw"
TZ = ZoneInfo("Europe/Moscow")


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
                # Насколько дороже среднего лечит врач; нужен для разных средних чеков.
                "_price_factor": float(np.clip(rng.normal(1, config.DOCTOR_PRICE_SD), 0.75, 1.3)),
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
                # Филиал, в который владелец обычно ходит.
                "_branch_id": int(rng.integers(1, len(config.BRANCHES) + 1)),
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


def lerp(bounds: tuple[float, float], t: float) -> float:
    """Значение между bounds[0] (при t=0) и bounds[1] (при t=1)."""
    return bounds[0] + (bounds[1] - bounds[0]) * t


class Calendar:
    """Все дни периода (и две недели после) с весом дня: сезонность × день недели."""

    def __init__(self) -> None:
        horizon = config.END + timedelta(days=config.SCHEDULE_AHEAD_DAYS)
        self.days = list(pd.date_range(config.START, horizon).date)
        self.ordinals = np.array([d.toordinal() for d in self.days])
        self.weights = np.array(
            [config.MONTH_WEIGHTS[d.month - 1] * config.WEEKDAY_WEIGHTS[d.weekday()] for d in self.days]
        )

    def pick_day(self, rng, first: date, last: date) -> date | None:
        """Случайный день из [first, last] с учётом весов."""
        lo = int(np.searchsorted(self.ordinals, first.toordinal(), side="left"))
        hi = int(np.searchsorted(self.ordinals, last.toordinal(), side="right"))
        if lo >= hi:
            return None
        w = self.weights[lo:hi]
        return self.days[lo + int(rng.choice(hi - lo, p=w / w.sum()))]


def pick_time(rng) -> tuple[int, int]:
    p = np.array(config.HOUR_WEIGHTS)
    hour = int(rng.choice(config.HOURS, p=p / p.sum()))
    return hour, int(rng.choice([0, 30]))


def pick_visit_type(rng, day: date, species: str) -> str:
    weights = dict(zip(config.VISIT_TYPES, config.VISIT_TYPE_WEIGHTS, strict=True))
    if day.month in (3, 4, 5):
        weights["vaccination"] *= config.VACCINATION_SPRING_BOOST
    if not config.VACCINES[species]:
        weights["vaccination"] = 0
    p = np.array(list(weights.values()))
    return str(rng.choice(list(weights), p=p / p.sum()))


def pick_lead_days(rng) -> int:
    """За сколько дней до приёма записались; 0 — в тот же день."""
    if rng.random() < config.SAME_DAY_SHARE:
        return 0
    return int(min(1 + rng.exponential(config.LEAD_DAYS_MEAN), config.LEAD_DAYS_MAX))


def no_show_probability(scheduled_at: datetime, lead_days: int, reminder: bool) -> float:
    p = config.NO_SHOW_BASE + config.NO_SHOW_PER_LEAD_DAY * lead_days
    if scheduled_at.weekday() == 0:
        p += config.NO_SHOW_MONDAY
    if (scheduled_at.hour, scheduled_at.minute) <= (10, 30):
        p += config.NO_SHOW_EARLY
    p = min(p, config.NO_SHOW_MAX)
    if reminder:
        p *= 1 - config.REMINDER_EFFECT
    return p


def pick_doctor(rng, doctors: list[dict], branch_id: int, day: date, visit_type: str) -> dict | None:
    """Врач, нанятый до дня приёма: сначала из филиала владельца, иначе из любого."""
    hired = [d for d in doctors if d["hired_at"] <= day]
    if visit_type == "surgery":
        hired = [d for d in hired if d["specialization"] == config.SURGEON]
    local = [d for d in hired if d["branch_id"] == branch_id]
    pool = local or hired
    if not pool:
        return None
    return pool[int(rng.integers(len(pool)))]


def make_visits(rng, owners, pets, medical_cards, doctors) -> pd.DataFrame:
    calendar = Calendar()
    horizon = config.END + timedelta(days=config.SCHEDULE_AHEAD_DAYS)
    card_of_pet = dict(zip(medical_cards["pet_id"], medical_cards["id"], strict=True))
    owner_by_id = owners.set_index("id").to_dict("index")
    doctor_list = doctors.to_dict("records")  # список словарей быстрее, чем фильтры DataFrame в цикле

    rows = []
    for pet in pets.itertuples():
        owner = owner_by_id[pet.owner_id]
        is_new_client = owner["registered_at"] >= config.START
        active_from = max(config.START, owner["registered_at"])

        # Отток: нелояльные клиенты уходят быстрее.
        churn_years = rng.exponential(lerp(config.CHURN_YEARS, owner["_loyalty"]))
        active_to = min(horizon, active_from + timedelta(days=int(churn_years * 365)))

        years = max((active_to - active_from).days, 0) / 365
        n_visits = int(rng.poisson(lerp(config.VISITS_PER_YEAR, owner["_loyalty"]) * years))

        days = []
        if is_new_client:  # новый клиент приходит вскоре после регистрации
            first_to = min(horizon, active_from + timedelta(days=config.FIRST_VISIT_DAYS))
            days.append(calendar.pick_day(rng, active_from, first_to))
        days += [calendar.pick_day(rng, active_from, active_to) for _ in range(n_visits)]

        for day in days:
            if day is None:
                continue
            hour, minute = pick_time(rng)
            scheduled_at = datetime(day.year, day.month, day.day, hour, minute, tzinfo=TZ)
            visit_type = pick_visit_type(rng, day, pet.species)
            doctor = pick_doctor(rng, doctor_list, owner["_branch_id"], day, visit_type)
            if doctor is None and visit_type == "surgery":
                visit_type = "treatment"
                doctor = pick_doctor(rng, doctor_list, owner["_branch_id"], day, visit_type)
            if doctor is None:
                continue

            lead_days = pick_lead_days(rng)
            if lead_days == 0:
                booked_at = scheduled_at - timedelta(minutes=int(rng.integers(30, 180)))
            else:
                booked_at = scheduled_at - timedelta(days=lead_days, minutes=int(rng.integers(0, 600)))

            # A/B-эксперимент: после старта напоминание получает случайная половина записей
            # (при записи в тот же день напоминать нечего).
            reminder = (
                day >= config.EXPERIMENT_START and lead_days > 0 and rng.random() < config.REMINDER_SHARE
            )

            if day > config.END:
                status = "scheduled"
            elif rng.random() < config.CANCEL_SHARE:
                status = "cancelled"
            elif rng.random() < no_show_probability(scheduled_at, lead_days, reminder):
                status = "no_show"
            else:
                status = "completed"

            completed = status == "completed"
            cabinet = "операционная" if visit_type == "surgery" else str(rng.integers(1, config.CABINETS + 1))
            rows.append(
                {
                    "medical_card_id": card_of_pet[pet.id],
                    "doctor_id": int(doctor["id"]),
                    "branch_id": int(doctor["branch_id"]),
                    "cabinet": cabinet,
                    "booked_at": booked_at,
                    "scheduled_at": scheduled_at,
                    "visit_type": visit_type,
                    "status": status,
                    "diagnosis": str(rng.choice(config.DIAGNOSES[visit_type])) if completed else None,
                    "recommendations": (
                        str(rng.choice(config.RECOMMENDATIONS[visit_type])) if completed else None
                    ),
                    "reminder_sent": reminder,
                    "_pet_id": pet.id,
                    "_species": pet.species,
                    "_size": pet.size,
                    "_owner_id": pet.owner_id,
                    "_lead_days": lead_days,
                }
            )

    visits = pd.DataFrame(rows).sort_values("scheduled_at", kind="stable").reset_index(drop=True)
    visits.insert(0, "id", range(1, len(visits) + 1))
    return visits


def make_payments(rng, visits: pd.DataFrame, doctors: pd.DataFrame, owners: pd.DataFrame) -> pd.DataFrame:
    price_factor = dict(zip(doctors["id"], doctors["_price_factor"], strict=True))
    loyalty = dict(zip(owners["id"], owners["_loyalty"], strict=True))
    methods_p = np.array(config.PAYMENT_METHOD_WEIGHTS)
    rows = []
    for visit in visits[visits["status"] == "completed"].to_dict("records"):
        price = (
            config.BASE_PRICES[visit["visit_type"]]
            * config.SIZE_PRICE_FACTOR[visit["_size"]]
            * price_factor[visit["doctor_id"]]
        )
        if (
            loyalty[visit["_owner_id"]] >= config.LOYAL_THRESHOLD
            and rng.random() < config.LOYAL_DISCOUNT_SHARE
        ):
            discount = config.LOYAL_DISCOUNT
        elif rng.random() < config.PROMO_SHARE:
            discount = config.PROMO_DISCOUNT
        else:
            discount = 0
        minutes = int(rng.integers(*config.VISIT_MINUTES))
        rows.append(
            {
                "id": len(rows) + 1,
                "visit_id": visit["id"],
                "service": visit["visit_type"],
                "amount": round(price / 50) * 50,  # цены в прайсе кратны 50 ₽
                "discount": discount,
                "payment_method": str(rng.choice(config.PAYMENT_METHODS, p=methods_p / methods_p.sum())),
                "paid_at": visit["scheduled_at"] + timedelta(minutes=minutes),
            }
        )
    return pd.DataFrame(rows)


def make_vaccinations(rng, visits: pd.DataFrame) -> pd.DataFrame:
    """Прививка — результат состоявшегося приёма типа vaccination."""
    done = visits[(visits["status"] == "completed") & (visits["visit_type"] == "vaccination")]
    rows = []
    for visit in done.to_dict("records"):
        rows.append(
            {
                "id": len(rows) + 1,
                "pet_id": visit["_pet_id"],
                "vaccine": str(rng.choice(config.VACCINES[visit["_species"]])),
                "vaccinated_at": visit["scheduled_at"].date(),
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
    visits = make_visits(rng, owners, pets, medical_cards, doctors)
    payments = make_payments(rng, visits, doctors, owners)
    vaccinations = make_vaccinations(rng, visits)
    return {
        "branches": branches,
        "doctors": doctors,
        "owners": owners,
        "pets": pets,
        "medical_cards": medical_cards,
        "vaccinations": vaccinations,
        "visits": visits,
        "payments": payments,
    }


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    for name, df in build().items():
        save(df, name)
        print(f"{name}: {len(df)}")


if __name__ == "__main__":
    main()
