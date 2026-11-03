-- Вопрос бизнеса: возвращаются ли клиенты после первого визита и как быстро мы их теряем?
--
-- Когорта — клиенты, впервые пришедшие в одном месяце. Retention (удержание) месяца N — доля
-- когорты, у которой был состоявшийся приём через N месяцев после месяца первого визита.
-- Месяц 0 по определению 100%.
--
-- Тонкость (цензурирование слева): у части клиентов история началась до начала данных —
-- их «первый визит» в данных на самом деле не первый, и они раздули бы ранние когорты.
-- Поэтому берём только новых клиентов — зарегистрированных не раньше первого дня данных.
-- Справа данные тоже обрезаны: у свежих когорт ещё нет поздних месяцев, их не показываем.
--
-- Пример (матрица когорт):
--   SELECT cohort_month, month_n, retention_pct FROM metrics.cohort_retention
--   WHERE month_n <= 12 ORDER BY cohort_month, month_n;

-- Новые клиенты с когортой.
CREATE VIEW metrics.new_owners AS
SELECT o.owner_id, o.cohort_month, o.pets_count
FROM mart.dim_owner AS o
WHERE o.cohort_month IS NOT NULL
  AND o.registered_at >= (SELECT first_date FROM metrics.period)
  AND o.cohort_month <= (SELECT last_full_month FROM metrics.period);

COMMENT ON VIEW metrics.new_owners IS 'Клиенты, вся история которых есть в данных, с месяцем когорты';


-- Сетка «когорта × номер месяца»: все месяцы от когорты до конца данных, даже пустые.
-- Без сетки месяцы без визитов просто пропали бы, и удержание выглядело бы выше.
CREATE VIEW metrics.cohort_grid AS
WITH cohorts AS (
    SELECT cohort_month, count(*) AS cohort_size
    FROM metrics.new_owners
    GROUP BY cohort_month
)
SELECT c.cohort_month,
       c.cohort_size,
       m.month::date AS month,
       (extract(year FROM age(m.month, c.cohort_month)) * 12
        + extract(month FROM age(m.month, c.cohort_month)))::int AS month_n
FROM cohorts AS c
CROSS JOIN LATERAL generate_series(
    c.cohort_month::timestamp,
    (SELECT last_full_month FROM metrics.period)::timestamp,
    interval '1 month'
) AS m(month);

COMMENT ON VIEW metrics.cohort_grid IS 'Все пары когорта × месяц до конца данных с номером месяца';


-- Активность: новый клиент × месяц, когда у него был состоявшийся приём, и выручка за месяц.
CREATE VIEW metrics.owner_monthly_activity AS
SELECT n.owner_id,
       n.cohort_month,
       d.month_start AS month,
       count(*) AS visits_completed,
       sum(f.revenue) AS revenue
FROM metrics.new_owners AS n
JOIN mart.fact_visits AS f ON f.owner_id = n.owner_id AND f.is_completed
JOIN mart.dim_date AS d ON d.date = f.visit_date
WHERE d.month_start <= (SELECT last_full_month FROM metrics.period)
GROUP BY n.owner_id, n.cohort_month, d.month_start;

COMMENT ON VIEW metrics.owner_monthly_activity IS 'Состоявшиеся приёмы и выручка нового клиента по месяцам';


CREATE VIEW metrics.cohort_retention AS
SELECT g.cohort_month,
       g.month_n,
       g.cohort_size,
       count(a.owner_id) AS active_owners,
       round(100.0 * count(a.owner_id) / g.cohort_size, 1) AS retention_pct
FROM metrics.cohort_grid AS g
LEFT JOIN metrics.owner_monthly_activity AS a
       ON a.cohort_month = g.cohort_month AND a.month = g.month
GROUP BY g.cohort_month, g.month_n, g.cohort_size;

COMMENT ON VIEW metrics.cohort_retention IS 'Удержание: доля когорты с приёмом в N-й месяц после первого';


-- Удержание по месяцам жизни, усреднённое по всем когортам с весом по размеру когорты.
-- Отвечает на вопрос «какая доля клиентов в среднем приходит в N-й месяц».
CREATE VIEW metrics.retention_curve AS
SELECT month_n,
       count(*) AS cohorts,
       sum(cohort_size) AS owners,
       round(100.0 * sum(active_owners) / sum(cohort_size), 1) AS retention_pct
FROM metrics.cohort_retention
GROUP BY month_n;

COMMENT ON VIEW metrics.retention_curve IS 'Средневзвешенное удержание по номеру месяца с первого визита';


-- Возвращаемость: доля клиентов, пришедших хотя бы раз ещё в течение 3, 6 и 12 месяцев.
-- Считаем только для когорт, у которых этот срок уже прошёл целиком.
CREATE VIEW metrics.repeat_rate AS
WITH horizons(months) AS (VALUES (3), (6), (12)),
returned AS (
    SELECT DISTINCT a.owner_id, a.cohort_month, g.month_n
    FROM metrics.owner_monthly_activity AS a
    JOIN metrics.cohort_grid AS g ON g.cohort_month = a.cohort_month AND g.month = a.month
    WHERE g.month_n > 0
)
SELECT h.months AS within_months,
       count(DISTINCT n.owner_id) AS owners,
       count(DISTINCT r.owner_id) AS returned_owners,
       round(100.0 * count(DISTINCT r.owner_id) / NULLIF(count(DISTINCT n.owner_id), 0), 1)
           AS repeat_pct
FROM horizons AS h
JOIN metrics.new_owners AS n
  ON n.cohort_month + make_interval(months => h.months) <= (SELECT last_full_month FROM metrics.period)
LEFT JOIN returned AS r
       ON r.owner_id = n.owner_id AND r.month_n <= h.months
GROUP BY h.months;

COMMENT ON VIEW metrics.repeat_rate IS 'Доля новых клиентов, вернувшихся в течение 3, 6 и 12 месяцев';
