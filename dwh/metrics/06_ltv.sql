-- Вопрос бизнеса: сколько денег приносит клиент за свою «жизнь» и за какой срок окупается
-- привлечение? Если привлечение клиента стоит X ₽, его стоит привлекать, пока LTV выше X.
--
-- LTV (lifetime value) месяца N — сколько в среднем принёс клиент когорты за месяцы 0…N.
-- Накопительная сумма — оконная функция SUM(...) OVER (PARTITION BY когорта ORDER BY месяц):
-- для каждой строки складывает все строки своей когорты до неё включительно.
-- Делим на размер всей когорты, а не на активных: ушедшие клиенты тоже часть средней.
--
-- Пример: SELECT * FROM metrics.ltv_curve WHERE month_n IN (0, 3, 6, 12);

CREATE VIEW metrics.cohort_ltv AS
WITH revenue_by_month AS (
    SELECT g.cohort_month,
           g.cohort_size,
           g.month_n,
           coalesce(sum(a.revenue), 0) AS revenue
    FROM metrics.cohort_grid AS g
    LEFT JOIN metrics.owner_monthly_activity AS a
           ON a.cohort_month = g.cohort_month AND a.month = g.month
    GROUP BY g.cohort_month, g.cohort_size, g.month_n
)
SELECT cohort_month,
       month_n,
       cohort_size,
       revenue,
       sum(revenue) OVER w AS cumulative_revenue,
       round(sum(revenue) OVER w / cohort_size, 2) AS ltv
FROM revenue_by_month
WINDOW w AS (PARTITION BY cohort_month ORDER BY month_n);

COMMENT ON VIEW metrics.cohort_ltv IS 'Выручка когорты по месяцам жизни и накопленный LTV на клиента';


-- Средний LTV по месяцу жизни по всем когортам, у которых этот месяц уже наступил.
-- Взвешиваем по размеру когорты: сумма накопленной выручки / сумма клиентов.
CREATE VIEW metrics.ltv_curve AS
SELECT month_n,
       count(*) AS cohorts,
       sum(cohort_size) AS owners,
       round(sum(cumulative_revenue) / sum(cohort_size), 2) AS ltv
FROM metrics.cohort_ltv
GROUP BY month_n;

COMMENT ON VIEW metrics.ltv_curve IS 'Средневзвешенный накопленный LTV по номеру месяца с первого визита';


-- LTV за первые 12 месяцев в зависимости от вида первого животного клиента.
-- Помогает решить, на каких клиентов тратить рекламный бюджет.
CREATE VIEW metrics.ltv_12m_by_species AS
WITH first_pet AS (
    SELECT DISTINCT ON (owner_id) owner_id, species
    FROM mart.dim_pet
    ORDER BY owner_id, pet_id
),
owner_12m AS (
    SELECT n.owner_id,
           coalesce(sum(a.revenue), 0) AS revenue_12m
    FROM metrics.new_owners AS n
    LEFT JOIN metrics.owner_monthly_activity AS a
           ON a.owner_id = n.owner_id
          AND a.month < n.cohort_month + interval '12 months'
    WHERE n.cohort_month + interval '11 months' <= (SELECT last_full_month FROM metrics.period)
    GROUP BY n.owner_id
)
SELECT fp.species,
       count(*) AS owners,
       round(avg(o.revenue_12m), 2) AS ltv_12m,
       round((percentile_cont(0.5) WITHIN GROUP (ORDER BY o.revenue_12m))::numeric, 2) AS median_12m
FROM owner_12m AS o
JOIN first_pet AS fp USING (owner_id)
GROUP BY fp.species;

COMMENT ON VIEW metrics.ltv_12m_by_species IS 'Средний и медианный LTV за 12 месяцев по виду первого животного';
