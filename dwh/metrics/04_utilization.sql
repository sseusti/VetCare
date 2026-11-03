-- Вопрос бизнеса: равномерно ли загружены врачи? Есть ли перегруженные и простаивающие?
--
-- Расписания слотов в данных нет, поэтому загрузку меряем как число состоявшихся приёмов
-- в рабочий день врача (рабочий день — день, когда у врача был хотя бы один приём в записи).
-- Сравниваем с медианой по клинике: percentile_cont(0.5) WITHIN GROUP (ORDER BY x) — медиана,
-- значение посередине. Она устойчивее среднего к выбросам.
--
-- Пример: SELECT * FROM metrics.doctor_load ORDER BY load_vs_median DESC;

-- Шаг 1: врач × день — сколько было записей и сколько приёмов состоялось.
CREATE VIEW metrics.doctor_daily AS
SELECT doctor_id,
       visit_date,
       count(*) FILTER (WHERE status <> 'cancelled') AS visits_booked,
       count(*) FILTER (WHERE is_completed) AS visits_completed
FROM mart.fact_visits
WHERE visit_date <= (SELECT last_date FROM metrics.period)
GROUP BY doctor_id, visit_date;

COMMENT ON VIEW metrics.doctor_daily IS 'Записи и состоявшиеся приёмы врача за день';


-- Шаг 2: агрегат по врачу и сравнение с медианой клиники.
CREATE VIEW metrics.doctor_load AS
WITH per_doctor AS (
    SELECT dd.doctor_id,
           count(*) AS working_days,
           sum(dd.visits_completed) AS visits_completed,
           round(avg(dd.visits_completed), 2) AS visits_per_day,
           max(dd.visits_completed) AS max_visits_per_day
    FROM metrics.doctor_daily AS dd
    GROUP BY dd.doctor_id
),
clinic AS (
    SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY visits_per_day) AS median_per_day
    FROM per_doctor
)
SELECT doc.short_name,
       doc.specialization,
       doc.branch_name,
       p.working_days,
       p.visits_completed,
       p.visits_per_day,
       p.max_visits_per_day,
       round((p.visits_per_day / NULLIF(c.median_per_day, 0))::numeric, 2) AS load_vs_median
FROM per_doctor AS p
CROSS JOIN clinic AS c
JOIN mart.dim_doctor AS doc USING (doctor_id);

COMMENT ON VIEW metrics.doctor_load IS
    'Рабочие дни, приёмы в день и загрузка относительно медианы клиники по врачу';


-- Загрузка по месяцам и филиалам: видно сезонные пики, когда врачей не хватает.
CREATE VIEW metrics.branch_load_monthly AS
SELECT d.month_start AS month,
       doc.branch_name,
       count(DISTINCT dd.doctor_id) AS doctors_working,
       sum(dd.visits_completed) AS visits_completed,
       round(sum(dd.visits_completed)::numeric / NULLIF(count(*), 0), 2) AS visits_per_doctor_day
FROM metrics.doctor_daily AS dd
JOIN mart.dim_date AS d ON d.date = dd.visit_date
JOIN mart.dim_doctor AS doc ON doc.doctor_id = dd.doctor_id
WHERE d.month_start <= (SELECT last_full_month FROM metrics.period)
GROUP BY d.month_start, doc.branch_name;

COMMENT ON VIEW metrics.branch_load_monthly IS 'Приёмы на врача в рабочий день по филиалу и месяцу';
