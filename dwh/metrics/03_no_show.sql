-- Вопрос бизнеса: когда и почему клиенты не приходят? Каждая неявка — пустой слот врача.
--
-- Доля неявок = неявки / (пришли + не пришли). Отмены не считаем: о них предупредили заранее.
-- Приёмы с SMS-напоминанием (эксперимент с марта 2026) сюда не берём: напоминание снижает
-- неявки и смешало бы эффекты. Сам эксперимент разбирается отдельно на шаге B6.
--
-- Примеры:
--   SELECT * FROM metrics.no_show_by_weekday;
--   SELECT * FROM metrics.no_show_by_lead_time;

-- Общая база для всех разрезов: прошедшие приёмы без напоминания, где клиент пришёл или нет.
CREATE VIEW metrics.no_show_base AS
SELECT f.*,
       d.weekday,
       d.weekday_name,
       d.month_start,
       p.species
FROM mart.fact_visits AS f
JOIN mart.dim_date AS d ON d.date = f.visit_date
JOIN mart.dim_pet AS p ON p.pet_id = f.pet_id
WHERE (f.is_completed OR f.is_no_show)
  AND NOT f.reminder_sent;

COMMENT ON VIEW metrics.no_show_base IS 'Прошедшие приёмы без напоминания: пришёл или не пришёл';


CREATE VIEW metrics.no_show_by_weekday AS
SELECT weekday,
       weekday_name,
       count(*) AS visits,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show) / count(*), 1) AS no_show_pct
FROM metrics.no_show_base
GROUP BY weekday, weekday_name;

COMMENT ON VIEW metrics.no_show_by_weekday IS 'Доля неявок по дню недели';


-- Срок записи — сколько дней прошло от записи до приёма. Корзины задаём через CASE.
CREATE VIEW metrics.no_show_by_lead_time AS
WITH bucketed AS (
    SELECT is_no_show,
           CASE
               WHEN lead_days <= 1 THEN 1
               WHEN lead_days <= 7 THEN 2
               WHEN lead_days <= 14 THEN 3
               ELSE 4
           END AS bucket_order
    FROM metrics.no_show_base
)
SELECT bucket_order,
       (ARRAY['0–1 день', '2–7 дней', '8–14 дней', '15+ дней'])[bucket_order] AS lead_time,
       count(*) AS visits,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show) / count(*), 1) AS no_show_pct
FROM bucketed
GROUP BY bucket_order;

COMMENT ON VIEW metrics.no_show_by_lead_time IS 'Доля неявок по сроку между записью и приёмом';


CREATE VIEW metrics.no_show_by_hour AS
SELECT visit_hour,
       count(*) AS visits,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show) / count(*), 1) AS no_show_pct
FROM metrics.no_show_base
GROUP BY visit_hour;

COMMENT ON VIEW metrics.no_show_by_hour IS 'Доля неявок по часу приёма (московское время)';


CREATE VIEW metrics.no_show_by_species AS
SELECT species,
       count(*) AS visits,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show) / count(*), 1) AS no_show_pct
FROM metrics.no_show_base
GROUP BY species;

COMMENT ON VIEW metrics.no_show_by_species IS 'Доля неявок по виду животного';


-- Сколько денег теряется на неявках: неявки × средний чек того же типа приёма.
-- Это оценка сверху: часть пустых слотов администратор успевает отдать другим клиентам.
CREATE VIEW metrics.no_show_lost_revenue AS
WITH avg_check AS (
    SELECT visit_type, avg(revenue) AS avg_check
    FROM mart.fact_visits
    WHERE is_completed
    GROUP BY visit_type
)
SELECT f.month_start AS month,
       count(*) AS no_shows,
       round(sum(a.avg_check), 2) AS lost_revenue_estimate
FROM mart.fact_visits AS v
JOIN mart.dim_date AS f ON f.date = v.visit_date
JOIN avg_check AS a USING (visit_type)
WHERE v.is_no_show
  AND f.month_start <= (SELECT last_full_month FROM metrics.period)
GROUP BY f.month_start;

COMMENT ON VIEW metrics.no_show_lost_revenue IS 'Неявки и оценка недополученной выручки по месяцам';
