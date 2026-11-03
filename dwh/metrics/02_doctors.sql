-- Вопрос бизнеса: кто из врачей приносит больше всего денег и как это выглядит внутри филиала?
--
-- RANK() OVER (PARTITION BY branch ORDER BY revenue DESC) — место врача в своём филиале.
-- PARTITION BY делит строки на группы, и окно считается в каждой группе отдельно.
-- Доля врача = его выручка / SUM(выручка) OVER (PARTITION BY филиал).
-- Неявки считаем от пришедших и не пришедших; отменённые не учитываем — клиент предупредил,
-- и слот можно было отдать другому.
--
-- Пример: SELECT * FROM metrics.doctor_performance ORDER BY branch_name, rank_in_branch;

CREATE VIEW metrics.doctor_performance AS
WITH per_doctor AS (
    SELECT doc.doctor_id,
           doc.short_name,
           doc.specialization,
           doc.branch_name,
           doc.is_new,
           count(*) FILTER (WHERE f.is_completed) AS visits_completed,
           count(*) FILTER (WHERE f.is_no_show) AS no_shows,
           count(DISTINCT f.owner_id) FILTER (WHERE f.is_completed) AS unique_owners,
           coalesce(sum(f.revenue), 0) AS revenue
    FROM mart.dim_doctor AS doc
    LEFT JOIN mart.fact_visits AS f
           ON f.doctor_id = doc.doctor_id
          AND f.visit_date <= (SELECT last_date FROM metrics.period)
    GROUP BY doc.doctor_id, doc.short_name, doc.specialization, doc.branch_name, doc.is_new
)
SELECT doctor_id,
       short_name,
       specialization,
       branch_name,
       is_new,
       visits_completed,
       unique_owners,
       revenue,
       round(revenue / NULLIF(visits_completed, 0), 2) AS avg_check,
       round(100.0 * no_shows / NULLIF(visits_completed + no_shows, 0), 1) AS no_show_pct,
       rank() OVER (PARTITION BY branch_name ORDER BY revenue DESC) AS rank_in_branch,
       rank() OVER (ORDER BY revenue DESC) AS rank_overall,
       round(100.0 * revenue / NULLIF(sum(revenue) OVER (PARTITION BY branch_name), 0), 1)
           AS branch_revenue_share_pct
FROM per_doctor;

COMMENT ON VIEW metrics.doctor_performance IS
    'Приёмы, выручка, средний чек и неявки по врачу; место и доля в своём филиале';


-- Вопрос бизнеса: есть ли разница в среднем чеке по специализациям?

CREATE VIEW metrics.specialization_summary AS
SELECT doc.specialization,
       count(DISTINCT doc.doctor_id) AS doctors,
       count(*) FILTER (WHERE f.is_completed) AS visits_completed,
       sum(f.revenue) AS revenue,
       round(sum(f.revenue) / NULLIF(count(*) FILTER (WHERE f.is_completed), 0), 2) AS avg_check,
       round(sum(f.revenue) / NULLIF(count(DISTINCT doc.doctor_id), 0), 2) AS revenue_per_doctor
FROM mart.fact_visits AS f
JOIN mart.dim_doctor AS doc ON doc.doctor_id = f.doctor_id
WHERE f.visit_date <= (SELECT last_date FROM metrics.period)
GROUP BY doc.specialization;

COMMENT ON VIEW metrics.specialization_summary IS 'Врачи, приёмы, выручка и средний чек по специализации';
