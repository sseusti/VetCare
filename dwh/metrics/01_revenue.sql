-- Вопрос бизнеса: как меняются выручка, поток клиентов и средний чек по месяцам?
--
-- Средний чек = выручка / число состоявшихся (оплаченных) приёмов.
-- MoM (month over month) — изменение к прошлому месяцу, YoY (year over year) — к тому же
-- месяцу год назад. YoY честнее MoM: он не путает рост с сезонностью.
-- LAG(x, n) — оконная функция: значение x из строки на n позиций раньше в порядке ORDER BY.
--
-- Пример: SELECT * FROM metrics.revenue_monthly;

CREATE VIEW metrics.revenue_monthly AS
WITH monthly AS (
    SELECT d.month_start AS month,
           count(*) FILTER (WHERE f.status <> 'scheduled') AS visits_booked,
           count(*) FILTER (WHERE f.is_completed) AS visits_completed,
           count(DISTINCT f.owner_id) FILTER (WHERE f.is_completed) AS active_owners,
           sum(f.revenue) AS revenue
    FROM mart.fact_visits AS f
    JOIN mart.dim_date AS d ON d.date = f.visit_date
    WHERE d.month_start <= (SELECT last_full_month FROM metrics.period)
    GROUP BY d.month_start
)
SELECT month,
       visits_booked,
       visits_completed,
       active_owners,
       revenue,
       round(revenue / NULLIF(visits_completed, 0), 2) AS avg_check,
       round(100.0 * (revenue / NULLIF(lag(revenue) OVER w, 0) - 1), 1) AS revenue_mom_pct,
       round(100.0 * (revenue / NULLIF(lag(revenue, 12) OVER w, 0) - 1), 1) AS revenue_yoy_pct,
       sum(revenue) OVER (PARTITION BY date_trunc('year', month) ORDER BY month) AS revenue_ytd
FROM monthly
WINDOW w AS (ORDER BY month);

COMMENT ON VIEW metrics.revenue_monthly IS
    'Выручка, приёмы, активные владельцы и средний чек по месяцам; MoM, YoY и накопленно с начала года';


-- Вопрос бизнеса: какие услуги приносят деньги — частые и дешёвые или редкие и дорогие?
-- Доля в выручке считается окном без PARTITION BY: SUM(...) OVER () — сумма по всем строкам.

CREATE VIEW metrics.revenue_by_visit_type AS
SELECT visit_type,
       count(*) FILTER (WHERE is_completed) AS visits_completed,
       sum(revenue) AS revenue,
       round(sum(revenue) / NULLIF(count(*) FILTER (WHERE is_completed), 0), 2) AS avg_check,
       round(100.0 * count(*) FILTER (WHERE is_completed)
             / sum(count(*) FILTER (WHERE is_completed)) OVER (), 1) AS visits_share_pct,
       round(100.0 * sum(revenue) / sum(sum(revenue)) OVER (), 1) AS revenue_share_pct
FROM mart.fact_visits
WHERE visit_date <= (SELECT last_date FROM metrics.period)
GROUP BY visit_type;

COMMENT ON VIEW metrics.revenue_by_visit_type IS 'Число приёмов, выручка, средний чек и доли по типу приёма';


-- Вопрос бизнеса: как растут филиалы и какой из них отстаёт?

CREATE VIEW metrics.revenue_by_branch_monthly AS
SELECT d.month_start AS month,
       b.branch_name,
       count(*) FILTER (WHERE f.is_completed) AS visits_completed,
       sum(f.revenue) AS revenue,
       round(sum(f.revenue) / NULLIF(count(*) FILTER (WHERE f.is_completed), 0), 2) AS avg_check,
       round(100.0 * sum(f.revenue) / sum(sum(f.revenue)) OVER (PARTITION BY d.month_start), 1)
           AS revenue_share_pct
FROM mart.fact_visits AS f
JOIN mart.dim_date AS d ON d.date = f.visit_date
JOIN mart.dim_branch AS b ON b.branch_id = f.branch_id
WHERE d.month_start <= (SELECT last_full_month FROM metrics.period)
GROUP BY d.month_start, b.branch_name;

COMMENT ON VIEW metrics.revenue_by_branch_monthly IS 'Выручка и доля филиала в выручке клиники по месяцам';
