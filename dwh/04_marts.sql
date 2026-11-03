-- Аналитическое хранилище: staging и витрины по схеме «звезда».
--
--   oltp (рабочая база)  -->  stg (представления: подготовка)  -->  mart (таблицы: звезда)
--
-- stg один раз решает «неудобные» вопросы: московское время, выручка после скидки,
-- связь приёма с животным и владельцем. Витрины строятся уже из stg.
-- Витрины — таблицы: их много раз читает дашборд, выгоднее посчитать один раз.
-- Скрипт можно запускать повторно: обе схемы пересоздаются.
--
-- Запуск:
--   docker compose exec -T postgres psql -U vetcare -d vetcare -v ON_ERROR_STOP=1 < dwh/04_marts.sql

\set ON_ERROR_STOP on

-- =====================================================================
-- 1. Staging
-- =====================================================================

DROP SCHEMA IF EXISTS mart CASCADE;
DROP SCHEMA IF EXISTS stg CASCADE;
CREATE SCHEMA stg;

-- Дата и час считаются по московскому времени: приём в 01:00 по Москве —
-- это 22:00 предыдущего дня по UTC, и без перевода он уехал бы в чужой день.
CREATE VIEW stg.visits AS
SELECT
    v.id AS visit_id,
    v.scheduled_at,
    (v.scheduled_at AT TIME ZONE 'Europe/Moscow')::date AS visit_date,
    extract(HOUR FROM v.scheduled_at AT TIME ZONE 'Europe/Moscow')::int AS visit_hour,
    (v.scheduled_at AT TIME ZONE 'Europe/Moscow')::date
        - (v.booked_at AT TIME ZONE 'Europe/Moscow')::date AS lead_days,
    v.visit_type,
    v.status,
    v.status = 'completed' AS is_completed,
    v.status = 'no_show' AS is_no_show,
    v.status = 'cancelled' AS is_cancelled,
    v.reminder_sent,
    v.doctor_id,
    v.branch_id,
    v.cabinet,
    mc.pet_id,
    p.owner_id
FROM oltp.visits AS v
JOIN oltp.medical_cards AS mc ON mc.id = v.medical_card_id
JOIN oltp.pets AS p ON p.id = mc.pet_id;

-- amount — цена до скидки, discount — скидка в процентах.
CREATE VIEW stg.payments AS
SELECT
    p.id AS payment_id,
    p.visit_id,
    p.paid_at,
    (p.paid_at AT TIME ZONE 'Europe/Moscow')::date AS paid_date,
    p.service,
    p.payment_method,
    p.amount,
    p.discount,
    round(p.amount * p.discount / 100, 2) AS discount_amount,
    round(p.amount * (1 - p.discount / 100), 2) AS revenue
FROM oltp.payments AS p;

-- =====================================================================
-- 2. Измерения: по ним фильтруют и группируют
-- =====================================================================

CREATE SCHEMA mart;

-- Календарь генерируется, а не берётся из данных: в нём есть все дни,
-- даже без приёмов, и атрибуты дня описаны в одном месте.
CREATE TABLE mart.dim_date AS
SELECT
    d::date AS date,
    extract(YEAR FROM d)::int AS year,
    extract(QUARTER FROM d)::int AS quarter,
    extract(MONTH FROM d)::int AS month,
    (ARRAY[
        'январь', 'февраль', 'март', 'апрель', 'май', 'июнь',
        'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'
    ])[extract(MONTH FROM d)::int] AS month_name,
    to_char(d, 'YYYY-MM') AS year_month,
    date_trunc('month', d)::date AS month_start,
    date_trunc('week', d)::date AS week_start,
    extract(ISODOW FROM d)::int AS weekday,  -- 1 = понедельник, 7 = воскресенье
    (ARRAY['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'])[extract(ISODOW FROM d)::int] AS weekday_name,
    extract(ISODOW FROM d) IN (6, 7) AS is_weekend,
    CASE
        WHEN extract(MONTH FROM d) IN (12, 1, 2) THEN 'зима'
        WHEN extract(MONTH FROM d) IN (3, 4, 5) THEN 'весна'
        WHEN extract(MONTH FROM d) IN (6, 7, 8) THEN 'лето'
        ELSE 'осень'
    END AS season
FROM generate_series(
    least((SELECT min(visit_date) FROM stg.visits), (SELECT min(paid_date) FROM stg.payments)),
    greatest((SELECT max(visit_date) FROM stg.visits), (SELECT max(paid_date) FROM stg.payments)),
    INTERVAL '1 day'
) AS d;

CREATE TABLE mart.dim_branch AS
SELECT id AS branch_id, name AS branch_name, address
FROM oltp.branches;

-- Новичок — врач, пришедший уже внутри анализируемого периода.
CREATE TABLE mart.dim_doctor AS
SELECT
    d.id AS doctor_id,
    concat_ws(' ', d.last_name, d.first_name, d.middle_name) AS full_name,
    d.last_name || ' ' || left(d.first_name, 1) || '.'
        || coalesce(' ' || left(d.middle_name, 1) || '.', '') AS short_name,
    d.specialization,
    d.branch_id,
    b.name AS branch_name,
    d.hired_at,
    d.hired_at > (SELECT min(visit_date) FROM stg.visits) AS is_new
FROM oltp.doctors AS d
JOIN oltp.branches AS b ON b.id = d.branch_id;

CREATE TABLE mart.dim_pet AS
SELECT
    p.id AS pet_id,
    p.owner_id,
    p.name,
    p.species,
    p.breed,
    p.size,
    p.sex,
    p.birth_date,
    mc.chronic_diseases IS NOT NULL AS has_chronic,
    mc.allergies IS NOT NULL AS has_allergy
FROM oltp.pets AS p
JOIN oltp.medical_cards AS mc ON mc.pet_id = p.id;

-- Когорта — месяц первого состоявшегося визита: основа анализа удержания.
-- У владельцев, ни разу не дошедших до приёма, когорты нет (NULL).
CREATE TABLE mart.dim_owner AS
SELECT
    o.id AS owner_id,
    o.registered_at,
    first_visit.first_visit_date,
    date_trunc('month', first_visit.first_visit_date)::date AS cohort_month,
    (SELECT count(*) FROM oltp.pets AS p WHERE p.owner_id = o.id) AS pets_count
FROM oltp.owners AS o
LEFT JOIN (
    SELECT owner_id, min(visit_date) AS first_visit_date
    FROM stg.visits
    WHERE is_completed
    GROUP BY owner_id
) AS first_visit ON first_visit.owner_id = o.id;

-- =====================================================================
-- 3. Факты: что считаем
-- =====================================================================

-- Зерно: одна строка — один приём (в любом статусе).
-- LEFT JOIN к оплатам: у неявок и отмен оплаты нет, но приём должен остаться в факте.
CREATE TABLE mart.fact_visits AS
SELECT
    v.visit_id,
    v.visit_date,
    v.doctor_id,
    v.branch_id,
    v.pet_id,
    v.owner_id,
    v.visit_type,
    v.status,
    v.is_completed,
    v.is_no_show,
    v.is_cancelled,
    v.reminder_sent,
    v.lead_days,
    v.visit_hour,
    extract(YEAR FROM age(v.visit_date, pt.birth_date))::int AS pet_age_years,
    coalesce(p.revenue, 0) AS revenue
FROM stg.visits AS v
JOIN oltp.pets AS pt ON pt.id = v.pet_id
LEFT JOIN stg.payments AS p ON p.visit_id = v.visit_id;

-- Зерно: одна строка — одна оплата. Дата — дата оплаты.
CREATE TABLE mart.fact_payments AS
SELECT
    p.payment_id,
    p.visit_id,
    p.paid_date,
    v.visit_date,
    v.doctor_id,
    v.branch_id,
    v.pet_id,
    v.owner_id,
    p.service,
    p.payment_method,
    p.amount,
    p.discount,
    p.discount_amount,
    p.revenue
FROM stg.payments AS p
JOIN stg.visits AS v ON v.visit_id = p.visit_id;

-- =====================================================================
-- 4. Ключи и индексы
-- =====================================================================
-- Первичный ключ факта заодно проверяет зерно: если JOIN размножил строки,
-- ADD PRIMARY KEY упадёт. Внешние ключи гарантируют, что у каждого факта
-- есть все измерения.

ALTER TABLE mart.dim_date ADD PRIMARY KEY (date);
ALTER TABLE mart.dim_branch ADD PRIMARY KEY (branch_id);
ALTER TABLE mart.dim_doctor ADD PRIMARY KEY (doctor_id);
ALTER TABLE mart.dim_pet ADD PRIMARY KEY (pet_id);
ALTER TABLE mart.dim_owner ADD PRIMARY KEY (owner_id);

ALTER TABLE mart.fact_visits
    ADD PRIMARY KEY (visit_id),
    ADD FOREIGN KEY (visit_date) REFERENCES mart.dim_date (date),
    ADD FOREIGN KEY (doctor_id) REFERENCES mart.dim_doctor (doctor_id),
    ADD FOREIGN KEY (branch_id) REFERENCES mart.dim_branch (branch_id),
    ADD FOREIGN KEY (pet_id) REFERENCES mart.dim_pet (pet_id),
    ADD FOREIGN KEY (owner_id) REFERENCES mart.dim_owner (owner_id);

ALTER TABLE mart.fact_payments
    ADD PRIMARY KEY (payment_id),
    ADD FOREIGN KEY (visit_id) REFERENCES mart.fact_visits (visit_id),
    ADD FOREIGN KEY (paid_date) REFERENCES mart.dim_date (date),
    ADD FOREIGN KEY (doctor_id) REFERENCES mart.dim_doctor (doctor_id),
    ADD FOREIGN KEY (branch_id) REFERENCES mart.dim_branch (branch_id),
    ADD FOREIGN KEY (pet_id) REFERENCES mart.dim_pet (pet_id),
    ADD FOREIGN KEY (owner_id) REFERENCES mart.dim_owner (owner_id);

CREATE INDEX ON mart.fact_visits (visit_date);
CREATE INDEX ON mart.fact_visits (doctor_id);
CREATE INDEX ON mart.fact_visits (branch_id);
CREATE INDEX ON mart.fact_visits (pet_id);
CREATE INDEX ON mart.fact_visits (owner_id);
CREATE INDEX ON mart.fact_payments (paid_date);
CREATE INDEX ON mart.fact_payments (doctor_id);
CREATE INDEX ON mart.fact_payments (branch_id);
CREATE INDEX ON mart.fact_payments (owner_id);
CREATE INDEX ON mart.dim_owner (cohort_month);

COMMENT ON TABLE mart.fact_visits IS 'Зерно: один приём в любом статусе';
COMMENT ON TABLE mart.fact_payments IS 'Зерно: одна оплата';
COMMENT ON COLUMN mart.fact_visits.revenue IS 'Выручка с приёма после скидки, 0 если оплаты нет';
COMMENT ON COLUMN mart.fact_payments.revenue IS 'amount за вычетом скидки, руб.';
COMMENT ON COLUMN mart.dim_owner.cohort_month IS 'Месяц первого состоявшегося визита';

ANALYZE mart.dim_date, mart.dim_branch, mart.dim_doctor, mart.dim_pet, mart.dim_owner,
    mart.fact_visits, mart.fact_payments;
