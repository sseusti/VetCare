-- Данные для дашборда в DataLens.
--
-- DataLens работает в облаке и не видит базу в локальном Docker, поэтому данные попадают туда
-- файлами: make export выгружает каждое представление ниже в dwh/export/<имя>.csv.
-- Подключение к файлам в DataLens принимает до 10 CSV — здесь их 7.
--
-- Главная таблица — dashboard_visits: один приём на строку, с русскими подписями и флагами 0/1.
-- BI-инструмент сам считает по ней суммы и доли в любом разрезе, и фильтры по периоду, филиалу
-- и врачу работают на всех чартах сразу. Остальные таблицы — то, что в BI посчитать трудно:
-- когорты, кривые удержания и LTV, статус клиента, доверительные интервалы эксперимента.
--
-- Пример: SELECT * FROM metrics.dashboard_visits LIMIT 10;

CREATE VIEW metrics.dashboard_visits AS
SELECT f.visit_id,
       f.visit_date,
       d.month_start AS month,
       d.year,
       d.weekday,
       d.weekday_name,
       d.season,
       f.visit_hour,
       b.branch_name,
       doc.short_name AS doctor,
       doc.specialization,
       CASE f.visit_type
           WHEN 'checkup' THEN 'осмотр'
           WHEN 'vaccination' THEN 'вакцинация'
           WHEN 'treatment' THEN 'лечение'
           WHEN 'surgery' THEN 'хирургия'
       END AS visit_type,
       CASE f.status
           WHEN 'completed' THEN 'состоялся'
           WHEN 'no_show' THEN 'неявка'
           WHEN 'cancelled' THEN 'отменён'
       END AS status,
       f.is_completed::int AS is_completed,
       f.is_no_show::int AS is_no_show,
       f.is_cancelled::int AS is_cancelled,
       f.revenue,
       f.lead_days,
       CASE
           WHEN f.lead_days = 0 THEN '0 — в тот же день'
           WHEN f.lead_days = 1 THEN '1 день'
           WHEN f.lead_days <= 7 THEN '2–7 дней'
           WHEN f.lead_days <= 14 THEN '8–14 дней'
           ELSE '15+ дней'
       END AS lead_time,
       p.species,
       f.owner_id,
       o.cohort_month,
       CASE
           WHEN f.visit_date < e.start_date OR f.lead_days = 0 THEN 'вне эксперимента'
           WHEN f.reminder_sent THEN 'тест (SMS)'
           ELSE 'контроль'
       END AS ab_group
FROM mart.fact_visits AS f
JOIN mart.dim_date AS d ON d.date = f.visit_date
JOIN mart.dim_branch AS b ON b.branch_id = f.branch_id
JOIN mart.dim_doctor AS doc ON doc.doctor_id = f.doctor_id
JOIN mart.dim_pet AS p ON p.pet_id = f.pet_id
JOIN mart.dim_owner AS o ON o.owner_id = f.owner_id
CROSS JOIN metrics.experiment_period AS e
WHERE f.visit_date <= e.end_date;

COMMENT ON VIEW metrics.dashboard_visits IS
    'Прошедшие приёмы с подписями для BI: дата, филиал, врач, тип, статус, флаги 0/1, выручка, группа A/B';


-- Матрица когорт: удержание и накопленный LTV в одной клетке «когорта × месяц жизни».
CREATE VIEW metrics.dashboard_cohorts AS
SELECT r.cohort_month,
       to_char(r.cohort_month, 'YYYY-MM') AS cohort,
       r.month_n,
       r.cohort_size,
       r.active_owners,
       r.retention_pct,
       l.ltv
FROM metrics.cohort_retention AS r
JOIN metrics.cohort_ltv AS l USING (cohort_month, month_n);

COMMENT ON VIEW metrics.dashboard_cohorts IS 'Когорта × месяц жизни: размер, активные, удержание, LTV';


-- Средние кривые жизненного цикла: удержание и LTV по месяцу с первого визита.
CREATE VIEW metrics.dashboard_lifecycle AS
SELECT r.month_n,
       r.owners,
       r.retention_pct,
       l.ltv
FROM metrics.retention_curve AS r
JOIN metrics.ltv_curve AS l USING (month_n);

COMMENT ON VIEW metrics.dashboard_lifecycle IS 'Средневзвешенные удержание и LTV по месяцу с первого визита';


-- Клиенты: статус оттока и сколько принесли — для сводки и списка «кому позвонить».
CREATE VIEW metrics.dashboard_owners AS
SELECT owner_id,
       cohort_month,
       CASE WHEN pets_count >= 3 THEN '3+' ELSE pets_count::text END AS pets,
       last_visit_date,
       days_since_last_visit,
       visits_completed,
       revenue,
       CASE status
           WHEN 'active' THEN 'активный'
           WHEN 'at_risk' THEN 'засыпает'
           WHEN 'churned' THEN 'ушёл'
       END AS status
FROM metrics.owner_status;

COMMENT ON VIEW metrics.dashboard_owners IS 'Клиент: статус (активный / засыпает / ушёл), последний визит, выручка';


-- Итог эксперимента с 95% доверительным интервалом доли неявок (нормальное приближение).
-- В BI интервал не посчитать, поэтому он готовится здесь.
CREATE VIEW metrics.dashboard_experiment AS
SELECT CASE ab_group WHEN 'treatment' THEN 'тест (SMS)' ELSE 'контроль' END AS ab_group,
       bookings,
       not_cancelled,
       no_shows,
       no_show_pct,
       round(no_show_pct - 196 * sqrt(no_show_pct / 100 * (1 - no_show_pct / 100) / not_cancelled), 2)
           AS ci_low_pct,
       round(no_show_pct + 196 * sqrt(no_show_pct / 100 * (1 - no_show_pct / 100) / not_cancelled), 2)
           AS ci_high_pct,
       cancelled_pct,
       revenue_per_booking
FROM metrics.experiment_summary;

COMMENT ON VIEW metrics.dashboard_experiment IS 'Группы эксперимента: неявки с 95% ДИ, отмены, выручка на запись';


-- Неделя × группа: доля неявок за неделю и накопленная с начала эксперимента.
CREATE VIEW metrics.dashboard_experiment_weekly AS
SELECT week_start,
       CASE ab_group WHEN 'treatment' THEN 'тест (SMS)' ELSE 'контроль' END AS ab_group,
       not_cancelled,
       no_shows,
       no_show_pct,
       round(100.0 * sum(no_shows) OVER w / sum(not_cancelled) OVER w, 2) AS cumulative_no_show_pct
FROM metrics.experiment_weekly
WINDOW w AS (PARTITION BY ab_group ORDER BY week_start);

COMMENT ON VIEW metrics.dashboard_experiment_weekly IS 'Неявки по неделям и накопленно по группам эксперимента';


-- Наивное и правильное сравнение рядом — иллюстрация ловушки на дашборде.
CREATE VIEW metrics.dashboard_experiment_naive AS
SELECT 'наивное: все приёмы' AS comparison,
       CASE WHEN naive_group = 'с SMS' THEN 'с SMS' ELSE 'без SMS' END AS sms,
       no_show_pct
FROM metrics.experiment_naive_comparison
UNION ALL
SELECT 'правильное: участники эксперимента',
       CASE ab_group WHEN 'treatment' THEN 'с SMS' ELSE 'без SMS' END,
       no_show_pct
FROM metrics.experiment_summary;

COMMENT ON VIEW metrics.dashboard_experiment_naive IS 'Доля неявок с SMS и без: наивное сравнение и правильное';
