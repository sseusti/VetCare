-- A/B-эксперимент: снижают ли SMS-напоминания долю неявок?
--
-- Дизайн. С начала эксперимента каждая запись, сделанная хотя бы за день до приёма, случайно
-- попадает в одну из двух групп: treatment — клиенту отправляют SMS-напоминание, control — нет.
-- Записи «в тот же день» в эксперимент не входят: напоминать о приёме через час бессмысленно,
-- SMS им не отправляют никогда.
--
-- Отсюда главная ловушка: сравнить «с SMS» и «без SMS» по всем приёмам нельзя. В «без SMS»
-- попадут записи в тот же день, у которых неявок и так мало, и контроль будет выглядеть лучше,
-- чем есть. Сравниваем только участников эксперимента (lead_days >= 1) — их разбил монетка.
--
-- Начало эксперимента не задаём руками, а берём из данных: первый день, когда ушло напоминание.
-- Подробный статистический анализ — в analysis/ab_sms_reminders.ipynb.
--
-- Примеры:
--   SELECT * FROM metrics.experiment_summary;
--   SELECT * FROM metrics.experiment_weekly ORDER BY week_start, ab_group;

CREATE VIEW metrics.experiment_period AS
SELECT min(visit_date) FILTER (WHERE reminder_sent) AS start_date,
       (SELECT last_date FROM metrics.period) AS end_date
FROM mart.fact_visits;

COMMENT ON VIEW metrics.experiment_period IS 'Первый и последний день эксперимента с SMS-напоминаниями';


-- Все прошедшие записи хотя бы за день: и до эксперимента (period = pre, для A/A-проверок
-- и оценки базового уровня), и во время (period = experiment, с группой).
CREATE VIEW metrics.experiment_visits AS
SELECT f.visit_id,
       f.visit_date,
       d.week_start,
       f.owner_id,
       f.doctor_id,
       f.branch_id,
       f.visit_type,
       f.lead_days,
       f.visit_hour,
       d.weekday,
       d.weekday = 1 AS is_monday,
       f.visit_hour <= 10 AS is_early_slot,
       CASE WHEN f.visit_date >= e.start_date THEN 'experiment' ELSE 'pre' END AS period,
       CASE
           WHEN f.visit_date < e.start_date THEN NULL
           WHEN f.reminder_sent THEN 'treatment'
           ELSE 'control'
       END AS ab_group,
       f.status,
       f.is_completed,
       f.is_no_show,
       f.is_cancelled,
       f.is_completed OR f.is_no_show AS is_attended_or_no_show,
       f.revenue
FROM mart.fact_visits AS f
JOIN mart.dim_date AS d ON d.date = f.visit_date
CROSS JOIN metrics.experiment_period AS e
WHERE f.lead_days >= 1
  AND f.visit_date <= e.end_date;

COMMENT ON VIEW metrics.experiment_visits IS
    'Записи хотя бы за день до приёма: период (pre / experiment), группа A/B, исход и ковариаты';


-- Итог по группам. Основная метрика — доля неявок среди тех, кто не отменил запись.
-- Барьерные метрики (guardrails) — то, что не должно ухудшиться: доля отмен и выручка на запись.
CREATE VIEW metrics.experiment_summary AS
SELECT ab_group,
       count(*) AS bookings,
       count(*) FILTER (WHERE is_attended_or_no_show) AS not_cancelled,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show)
             / NULLIF(count(*) FILTER (WHERE is_attended_or_no_show), 0), 2) AS no_show_pct,
       round(100.0 * count(*) FILTER (WHERE is_cancelled) / count(*), 2) AS cancelled_pct,
       round(sum(revenue) / count(*), 2) AS revenue_per_booking,
       count(DISTINCT owner_id) AS owners
FROM metrics.experiment_visits
WHERE period = 'experiment'
GROUP BY ab_group;

COMMENT ON VIEW metrics.experiment_summary IS
    'Группы эксперимента: записи, неявки, отмены и выручка на запись';


-- Наивное сравнение — как НЕ надо. Нужно для дашборда и объяснения ловушки.
CREATE VIEW metrics.experiment_naive_comparison AS
SELECT CASE WHEN f.reminder_sent THEN 'с SMS' ELSE 'без SMS (включая запись в тот же день)' END AS naive_group,
       count(*) FILTER (WHERE f.is_completed OR f.is_no_show) AS not_cancelled,
       round(100.0 * count(*) FILTER (WHERE f.is_no_show)
             / NULLIF(count(*) FILTER (WHERE f.is_completed OR f.is_no_show), 0), 2) AS no_show_pct
FROM mart.fact_visits AS f
CROSS JOIN metrics.experiment_period AS e
WHERE f.visit_date BETWEEN e.start_date AND e.end_date
GROUP BY 1;

COMMENT ON VIEW metrics.experiment_naive_comparison IS
    'Наивное сравнение «с SMS / без SMS» по всем приёмам — смещено записями в тот же день';


-- Неявки по неделям в каждой группе — видно, что эффект стабилен, а не держится на одной неделе.
CREATE VIEW metrics.experiment_weekly AS
SELECT week_start,
       ab_group,
       count(*) FILTER (WHERE is_attended_or_no_show) AS not_cancelled,
       count(*) FILTER (WHERE is_no_show) AS no_shows,
       round(100.0 * count(*) FILTER (WHERE is_no_show)
             / NULLIF(count(*) FILTER (WHERE is_attended_or_no_show), 0), 2) AS no_show_pct
FROM metrics.experiment_visits
WHERE period = 'experiment'
GROUP BY week_start, ab_group;

COMMENT ON VIEW metrics.experiment_weekly IS 'Доля неявок по неделям и группам эксперимента';
