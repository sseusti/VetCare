-- Загрузка CSV из генератора в рабочую базу клиники.
--
--   data-generator/data/raw/*.csv  --\copy-->  схема raw (всё TEXT)  --INSERT ... SELECT-->  схема oltp
--
-- raw — «прихожая»: файл ложится как есть и загрузка не падает на типах.
-- Типы приводятся при переносе в oltp, где работают все ограничения схемы.
-- Скрипт можно запускать повторно: raw пересоздаётся, oltp очищается перед загрузкой.
--
-- Запуск (CSV смонтированы в контейнер в /data/raw, см. docker-compose.yml):
--   docker compose exec -T postgres psql -U vetcare -d vetcare -v ON_ERROR_STOP=1 < dwh/02_load.sql

\set ON_ERROR_STOP on

-- ---- 1. Сырые таблицы: колонки как в CSV, всё текстом ----

DROP SCHEMA IF EXISTS raw CASCADE;
CREATE SCHEMA raw;

CREATE TABLE raw.branches (id TEXT, name TEXT, address TEXT);
CREATE TABLE raw.doctors (
    id TEXT, last_name TEXT, first_name TEXT, middle_name TEXT, phone TEXT,
    specialization TEXT, branch_id TEXT, hired_at TEXT
);
CREATE TABLE raw.owners (
    id TEXT, last_name TEXT, first_name TEXT, middle_name TEXT, phone TEXT, registered_at TEXT
);
CREATE TABLE raw.pets (
    id TEXT, owner_id TEXT, name TEXT, species TEXT, breed TEXT, color TEXT,
    sex TEXT, size TEXT, birth_date TEXT
);
CREATE TABLE raw.medical_cards (
    id TEXT, pet_id TEXT, chronic_diseases TEXT, allergies TEXT, created_at TEXT
);
CREATE TABLE raw.vaccinations (id TEXT, pet_id TEXT, vaccine TEXT, vaccinated_at TEXT);
CREATE TABLE raw.visits (
    id TEXT, medical_card_id TEXT, doctor_id TEXT, branch_id TEXT, cabinet TEXT,
    booked_at TEXT, scheduled_at TEXT, visit_type TEXT, status TEXT,
    diagnosis TEXT, recommendations TEXT, reminder_sent TEXT
);
CREATE TABLE raw.payments (
    id TEXT, visit_id TEXT, service TEXT, amount TEXT, discount TEXT,
    payment_method TEXT, paid_at TEXT
);

\copy raw.branches FROM '/data/raw/branches.csv' CSV HEADER
\copy raw.doctors FROM '/data/raw/doctors.csv' CSV HEADER
\copy raw.owners FROM '/data/raw/owners.csv' CSV HEADER
\copy raw.pets FROM '/data/raw/pets.csv' CSV HEADER
\copy raw.medical_cards FROM '/data/raw/medical_cards.csv' CSV HEADER
\copy raw.vaccinations FROM '/data/raw/vaccinations.csv' CSV HEADER
\copy raw.visits FROM '/data/raw/visits.csv' CSV HEADER
\copy raw.payments FROM '/data/raw/payments.csv' CSV HEADER

-- ---- 2. Перенос в oltp: одна транзакция — либо всё, либо ничего ----

BEGIN;

-- CASCADE очищает и все таблицы, ссылающиеся на перечисленные, поэтому порядок тут не важен.
-- RESTART IDENTITY сбрасывает счётчики id.
TRUNCATE oltp.branches, oltp.owners RESTART IDENTITY CASCADE;

-- Дальше порядок важен: сначала таблицы, на которые ссылаются другие.
-- OVERRIDING SYSTEM VALUE разрешает вставить свои id в колонку GENERATED ALWAYS:
-- без этого связи из генератора (приём -> врач №5) развалились бы.

INSERT INTO oltp.branches (id, name, address)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, name, address
FROM raw.branches;

INSERT INTO oltp.doctors (id, last_name, first_name, middle_name, phone, specialization, branch_id, hired_at)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, last_name, first_name, middle_name, phone, specialization, branch_id::bigint, hired_at::date
FROM raw.doctors;

INSERT INTO oltp.owners (id, last_name, first_name, middle_name, phone, registered_at)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, last_name, first_name, middle_name, phone, registered_at::date
FROM raw.owners;

INSERT INTO oltp.pets (id, owner_id, name, species, breed, color, sex, size, birth_date)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, owner_id::bigint, name, species, breed, color, sex, size, birth_date::date
FROM raw.pets;

INSERT INTO oltp.medical_cards (id, pet_id, chronic_diseases, allergies, created_at)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, pet_id::bigint, chronic_diseases, allergies, created_at::date
FROM raw.medical_cards;

INSERT INTO oltp.vaccinations (id, pet_id, vaccine, vaccinated_at)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, pet_id::bigint, vaccine, vaccinated_at::date
FROM raw.vaccinations;

INSERT INTO oltp.visits (
    id, medical_card_id, doctor_id, branch_id, cabinet, booked_at, scheduled_at,
    visit_type, status, diagnosis, recommendations, reminder_sent
)
OVERRIDING SYSTEM VALUE
SELECT
    id::bigint, medical_card_id::bigint, doctor_id::bigint, branch_id::bigint, cabinet,
    booked_at::timestamptz, scheduled_at::timestamptz,
    visit_type, status, diagnosis, recommendations, reminder_sent::boolean
FROM raw.visits;

INSERT INTO oltp.payments (id, visit_id, service, amount, discount, payment_method, paid_at)
OVERRIDING SYSTEM VALUE
SELECT id::bigint, visit_id::bigint, service, amount::numeric, discount::numeric, payment_method, paid_at::timestamptz
FROM raw.payments;

-- ---- 3. Счётчики id ----
-- Мы вставили свои id, а счётчик identity об этом не знает и стоит на 1:
-- следующий обычный INSERT получил бы id = 1 и упал на дубликате ключа.
-- Ставим каждый счётчик на текущий максимум (для пустой таблицы — на 1, следующий id тоже 1).

SELECT setval(pg_get_serial_sequence('oltp.' || t, 'id'), m, m IS NOT NULL)
FROM (
    VALUES
        ('branches', (SELECT max(id) FROM oltp.branches)),
        ('doctors', (SELECT max(id) FROM oltp.doctors)),
        ('owners', (SELECT max(id) FROM oltp.owners)),
        ('pets', (SELECT max(id) FROM oltp.pets)),
        ('medical_cards', (SELECT max(id) FROM oltp.medical_cards)),
        ('vaccinations', (SELECT max(id) FROM oltp.vaccinations)),
        ('visits', (SELECT max(id) FROM oltp.visits)),
        ('payments', (SELECT max(id) FROM oltp.payments))
) AS s (t, m);

COMMIT;

-- Обновить статистику планировщика после массовой загрузки.
ANALYZE;
