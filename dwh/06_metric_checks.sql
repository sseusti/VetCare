-- Проверки метрик: суммы сходятся с витринами, доли и проценты в своих границах,
-- накопительные величины не убывают. В здоровых данных везде 0; иначе скрипт падает.
--
-- Запуск (после make metrics):
--   docker compose exec -T postgres psql -U vetcare -d vetcare -v ON_ERROR_STOP=1 < dwh/06_metric_checks.sql

\set ON_ERROR_STOP on

CREATE TEMP TABLE metric_checks AS

SELECT 'revenue_monthly: выручка = выручке витрины за полные месяцы' AS check_name,
       CASE WHEN (SELECT sum(revenue) FROM metrics.revenue_monthly)
                 = (SELECT sum(f.revenue)
                    FROM mart.fact_visits AS f
                    JOIN mart.dim_date AS d ON d.date = f.visit_date
                    WHERE d.month_start <= (SELECT last_full_month FROM metrics.period))
            THEN 0 ELSE 1 END AS problems

UNION ALL
SELECT 'revenue_monthly: нет пропущенных месяцев',
       (SELECT (extract(year FROM age(max(month), min(month))) * 12
                + extract(month FROM age(max(month), min(month))) + 1)::int - count(*)
        FROM metrics.revenue_monthly)

UNION ALL
SELECT 'revenue_by_visit_type: доли в выручке дают 100%',
       CASE WHEN abs((SELECT sum(revenue_share_pct) FROM metrics.revenue_by_visit_type) - 100) <= 0.5
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'revenue_by_branch_monthly: доли филиалов в месяце дают 100%', count(*)
FROM (
    SELECT month FROM metrics.revenue_by_branch_monthly
    GROUP BY month HAVING abs(sum(revenue_share_pct) - 100) > 0.5
) AS bad

UNION ALL
SELECT 'doctor_performance: выручка врачей = выручке клиники',
       CASE WHEN (SELECT sum(revenue) FROM metrics.doctor_performance)
                 = (SELECT sum(revenue) FROM mart.fact_visits
                    WHERE visit_date <= (SELECT last_date FROM metrics.period))
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'doctor_performance: в каждом филиале есть врач на 1-м месте', count(*)
FROM (
    SELECT branch_name FROM metrics.doctor_performance
    GROUP BY branch_name HAVING count(*) FILTER (WHERE rank_in_branch = 1) = 0
) AS bad

UNION ALL
SELECT 'no_show: все 7 дней недели', 7 - (SELECT count(*) FROM metrics.no_show_by_weekday)

UNION ALL
SELECT 'no_show: разрезы покрывают всю базу', count(*)
FROM (
    SELECT sum(visits) AS visits FROM metrics.no_show_by_weekday
    UNION ALL SELECT sum(visits) FROM metrics.no_show_by_lead_time
    UNION ALL SELECT sum(visits) FROM metrics.no_show_by_hour
    UNION ALL SELECT sum(visits) FROM metrics.no_show_by_species
) AS s
WHERE s.visits <> (SELECT count(*) FROM metrics.no_show_base)

UNION ALL
SELECT 'doctor_load: приёмы врачей = состоявшимся приёмам',
       CASE WHEN (SELECT sum(visits_completed) FROM metrics.doctor_load)
                 = (SELECT count(*) FROM mart.fact_visits WHERE is_completed)
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'cohort_retention: в месяц 0 удержание 100%', count(*)
FROM metrics.cohort_retention
WHERE month_n = 0 AND retention_pct <> 100

UNION ALL
SELECT 'cohort_retention: удержание от 0 до 100%', count(*)
FROM metrics.cohort_retention
WHERE retention_pct NOT BETWEEN 0 AND 100

UNION ALL
SELECT 'cohort_grid: месяцы когорты идут подряд с нуля', count(*)
FROM (
    SELECT cohort_month FROM metrics.cohort_grid
    GROUP BY cohort_month HAVING min(month_n) <> 0 OR max(month_n) + 1 <> count(*)
) AS bad

UNION ALL
SELECT 'cohort_ltv: LTV не убывает внутри когорты', count(*)
FROM (
    SELECT ltv, lag(ltv) OVER (PARTITION BY cohort_month ORDER BY month_n) AS prev_ltv
    FROM metrics.cohort_ltv
) AS s
WHERE s.ltv < s.prev_ltv

UNION ALL
SELECT 'cohort_ltv: накопленная выручка = выручке новых клиентов',
       CASE WHEN (SELECT sum(revenue) FROM metrics.cohort_ltv)
                 = (SELECT sum(revenue) FROM metrics.owner_monthly_activity)
            THEN 0 ELSE 1 END

UNION ALL
SELECT 'owner_status: статус есть у каждого клиента с визитом',
       abs((SELECT count(*) FROM metrics.owner_status)
           - (SELECT count(DISTINCT owner_id) FROM mart.fact_visits WHERE is_completed))

UNION ALL
SELECT 'churn_summary: доли статусов дают 100%',
       CASE WHEN abs((SELECT sum(owners_pct) FROM metrics.churn_summary) - 100) <= 0.5
            THEN 0 ELSE 1 END;

SELECT check_name, problems, CASE WHEN problems = 0 THEN 'OK' ELSE 'FAIL' END AS result
FROM metric_checks;

DO $$
DECLARE
    failed int;
BEGIN
    SELECT count(*) INTO failed FROM metric_checks WHERE problems <> 0;
    IF failed > 0 THEN
        RAISE EXCEPTION 'Не пройдено проверок метрик: %', failed;
    END IF;
    RAISE NOTICE 'Все проверки метрик пройдены';
END
$$;
