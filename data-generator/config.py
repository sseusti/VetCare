from datetime import date

SEED = 42
START = date(2024, 11, 1)
END = date(2026, 10, 31)

BRANCHES = [
    ("VetCare Центр", "Нижний Новгород, ул. Большая Покровская, 12"),
    ("VetCare Сормово", "Нижний Новгород, ул. Коминтерна, 105"),
    ("VetCare Автозавод", "Нижний Новгород, пр. Ленина, 88"),
]

N_DOCTORS = 14
N_NEW_DOCTORS = 3
NEW_DOCTOR_DAYS = (30, 365)

SPECIALIZATIONS = ["терапевт", "хирург", "дерматолог", "офтальмолог", "кардиолог"]
SPECIALIZATION_WEIGHTS = [0.5, 0.2, 0.1, 0.1, 0.1]
