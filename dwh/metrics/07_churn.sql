-- Вопрос бизнеса: сколько клиентов мы уже потеряли и кого из «засыпающих» стоит вернуть?
--
-- Отток (churn) в клинике нельзя увидеть напрямую: клиент не расторгает договор, он просто
-- перестаёт приходить. Поэтому статус определяем по давности последнего визита на конец данных:
--   active   — был не позже 180 дней назад;
--   at_risk  — 181–365 дней: пора напомнить о себе (рассылка, скидка на осмотр);
--   churned  — больше 365 дней: скорее всего ушёл.
-- Пороги — бизнес-договорённость, их стоит обсуждать с заказчиком. Плановый осмотр раз в год,
-- поэтому меньше года без визита ещё не отток.
--
-- Пример: SELECT * FROM metrics.churn_summary;

CREATE VIEW metrics.owner_status AS
WITH last_visit AS (
    SELECT owner_id,
           max(visit_date) AS last_visit_date,
           count(*) AS visits_completed,
           sum(revenue) AS revenue
    FROM mart.fact_visits
    WHERE is_completed
    GROUP BY owner_id
)
SELECT o.owner_id,
       o.cohort_month,
       o.pets_count,
       lv.last_visit_date,
       lv.visits_completed,
       lv.revenue,
       p.last_date - lv.last_visit_date AS days_since_last_visit,
       CASE
           WHEN p.last_date - lv.last_visit_date <= 180 THEN 'active'
           WHEN p.last_date - lv.last_visit_date <= 365 THEN 'at_risk'
           ELSE 'churned'
       END AS status
FROM mart.dim_owner AS o
JOIN last_visit AS lv USING (owner_id)
CROSS JOIN metrics.period AS p;

COMMENT ON VIEW metrics.owner_status IS 'Последний визит владельца и статус active / at_risk / churned';


CREATE VIEW metrics.churn_summary AS
SELECT status,
       count(*) AS owners,
       round(100.0 * count(*) / sum(count(*)) OVER (), 1) AS owners_pct,
       sum(revenue) AS lifetime_revenue,
       round(avg(revenue), 2) AS avg_lifetime_revenue
FROM metrics.owner_status
GROUP BY status;

COMMENT ON VIEW metrics.churn_summary IS 'Сколько владельцев в каждом статусе и сколько они принесли';


-- Связь оттока с числом животных: у кого несколько питомцев, тот привязан к клинике сильнее?
-- Учитываем только клиентов, которые с нами хотя бы год, иначе новички занижают отток.
CREATE VIEW metrics.churn_by_pets AS
SELECT CASE WHEN pets_count >= 3 THEN '3+' ELSE pets_count::text END AS pets,
       count(*) AS owners,
       round(100.0 * count(*) FILTER (WHERE status = 'churned') / count(*), 1) AS churned_pct,
       round(100.0 * count(*) FILTER (WHERE status = 'at_risk') / count(*), 1) AS at_risk_pct
FROM metrics.owner_status
WHERE cohort_month <= (SELECT last_full_month FROM metrics.period) - interval '12 months'
GROUP BY 1;

COMMENT ON VIEW metrics.churn_by_pets IS 'Доля ушедших и засыпающих клиентов по числу животных';


-- Список для удержания: засыпающие клиенты, отсортированные по тому, сколько они приносили.
-- Это готовая выгрузка для администратора — кого обзвонить первым.
CREATE VIEW metrics.at_risk_owners AS
SELECT owner_id,
       last_visit_date,
       days_since_last_visit,
       visits_completed,
       revenue,
       rank() OVER (ORDER BY revenue DESC) AS priority
FROM metrics.owner_status
WHERE status = 'at_risk';

COMMENT ON VIEW metrics.at_risk_owners IS 'Засыпающие клиенты по убыванию выручки — кому напомнить первым';
