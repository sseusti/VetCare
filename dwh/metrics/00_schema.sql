-- Схема metrics: представления с бизнес-метриками поверх витрин mart.
-- Представление (VIEW) — сохранённый запрос: данных не хранит, считается при каждом обращении.
-- Поэтому метрики всегда совпадают с текущими витринами, а DataLens (шаг B7) читает их как таблицы.
--
-- Важно: 04_marts.sql пересоздаёт mart через DROP ... CASCADE и вместе с ним удаляет эти
-- представления. После пересборки витрин метрики нужно собрать заново: make metrics.
--
-- Запуск всех файлов по порядку: make metrics

DROP SCHEMA IF EXISTS metrics CASCADE;
CREATE SCHEMA metrics;

-- Отчётный период. Приёмы со статусом scheduled лежат в будущем, поэтому последний день
-- данных — последний день с прошедшими приёмами. Неполный последний месяц в помесячные
-- метрики не берём, иначе он выглядел бы как резкое падение.
CREATE VIEW metrics.period AS
WITH bounds AS (
    SELECT min(visit_date) AS first_date,
           max(visit_date) FILTER (WHERE status <> 'scheduled') AS last_date
    FROM mart.fact_visits
)
SELECT first_date,
       last_date,
       date_trunc('month', first_date)::date AS first_month,
       (date_trunc('month', last_date + 1) - interval '1 month')::date AS last_full_month
FROM bounds;

COMMENT ON VIEW metrics.period IS 'Границы данных: первый и последний день, последний полный месяц';
