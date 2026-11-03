-- Проверки витрин: факты полные, выручка сходится с oltp, измерения покрывают факты.
-- В здоровых данных везде 0; при нарушениях скрипт завершается с ошибкой.
--
-- Запуск:
--   docker compose exec -T postgres psql -U vetcare -d vetcare -v ON_ERROR_STOP=1 < dwh/05_mart_checks.sql

\set ON_ERROR_STOP on

CREATE TEMP TABLE checks AS

SELECT 'fact_visits: строк столько же, сколько приёмов' AS check_name,
       abs((SELECT count(*) FROM mart.fact_visits) - (SELECT count(*) FROM oltp.visits)) AS problems

UNION ALL
SELECT 'fact_payments: строк столько же, сколько оплат',
       abs((SELECT count(*) FROM mart.fact_payments) - (SELECT count(*) FROM oltp.payments))

UNION ALL
SELECT 'выручка в fact_visits = выручке в fact_payments',
       CASE WHEN (SELECT sum(revenue) FROM mart.fact_visits) = (SELECT sum(revenue) FROM mart.fact_payments)
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'выручка в витрине = ручному расчёту по oltp',
       CASE WHEN (SELECT sum(revenue) FROM mart.fact_payments)
                 = (SELECT sum(round(amount * (1 - discount / 100), 2)) FROM oltp.payments)
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'выручка только у состоявшихся приёмов', count(*)
FROM mart.fact_visits
WHERE revenue > 0 AND NOT is_completed

UNION ALL
SELECT 'неявок в витрине столько же, сколько в oltp',
       abs((SELECT count(*) FROM mart.fact_visits WHERE is_no_show)
           - (SELECT count(*) FROM oltp.visits WHERE status = 'no_show'))

UNION ALL
SELECT 'выручка по месяцам совпадает с oltp', count(*)
FROM (
    SELECT d.year_month, sum(f.revenue) AS revenue
    FROM mart.fact_payments AS f
    JOIN mart.dim_date AS d ON d.date = f.paid_date
    GROUP BY d.year_month
) AS m
FULL JOIN (
    SELECT to_char(paid_at AT TIME ZONE 'Europe/Moscow', 'YYYY-MM') AS year_month,
           sum(round(amount * (1 - discount / 100), 2)) AS revenue
    FROM oltp.payments
    GROUP BY 1
) AS o USING (year_month)
WHERE m.revenue IS DISTINCT FROM o.revenue

UNION ALL
SELECT 'в календаре нет пропущенных дней',
       (SELECT (max(date) - min(date) + 1) - count(*) FROM mart.dim_date)

UNION ALL
SELECT 'у владельца с состоявшимся визитом есть когорта', count(*)
FROM mart.dim_owner AS o
WHERE o.cohort_month IS NULL
  AND EXISTS (SELECT 1 FROM mart.fact_visits AS f WHERE f.owner_id = o.owner_id AND f.is_completed)

UNION ALL
SELECT 'первый визит не раньше регистрации', count(*)
FROM mart.dim_owner
WHERE first_visit_date < registered_at

UNION ALL
SELECT 'дата приёма в fact_visits по московскому времени', count(*)
FROM mart.fact_visits AS f
JOIN oltp.visits AS v ON v.id = f.visit_id
WHERE f.visit_date <> (v.scheduled_at AT TIME ZONE 'Europe/Moscow')::date
;

SELECT check_name AS "проверка", problems AS "нарушений",
       CASE WHEN problems = 0 THEN 'OK' ELSE 'ОШИБКА' END AS "итог"
FROM checks;

DO $$
DECLARE
    failed int;
BEGIN
    SELECT count(*) INTO failed FROM checks WHERE problems > 0;
    IF failed > 0 THEN
        RAISE EXCEPTION 'Проверок витрин с нарушениями: %', failed;
    END IF;
    RAISE NOTICE 'Все проверки витрин пройдены';
END
$$;
