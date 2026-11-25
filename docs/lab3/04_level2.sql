-- 2.1а. Пилоты с лимитом как у одного из нижегородских штурманов.
SELECT p.surname, p.hour_limit
FROM pilot AS p
WHERE p.hour_limit IN (
    SELECT n.hour_limit
    FROM navigator AS n
    WHERE n.squadron = 'Нижегородский'
)
ORDER BY p.surname, p.pilot_id;

-- 2.1б.1. Запрос 1.7а через IN.
SELECT DISTINCT n.surname
FROM navigator AS n
WHERE n.navigator_id IN (
    SELECT f.navigator_id
    FROM flight AS f
    JOIN route AS r USING (route_id)
    JOIN pilot AS p USING (pilot_id)
    WHERE r.destination = 'Хабаровск' OR p.squadron = n.squadron
)
ORDER BY n.surname;

-- 2.1б.2. Запрос 1.7б через IN.
SELECT DISTINCT p.surname, p.hour_limit
FROM pilot AS p
WHERE p.pilot_id IN (
    SELECT f.pilot_id
    FROM flight AS f
    JOIN route AS r USING (route_id)
    WHERE r.duration_hours > 3 AND f.flight_month >= 2
)
ORDER BY p.surname, p.hour_limit;

-- 2.1в. Пилоты, не летавшие в Хабаровск до декабря.
SELECT p.surname
FROM pilot AS p
WHERE p.pilot_id NOT IN (
    SELECT f.pilot_id
    FROM flight AS f
    JOIN route AS r USING (route_id)
    WHERE r.destination = 'Хабаровск' AND f.flight_month < 12
)
ORDER BY p.surname, p.pilot_id;

-- 2.2а. Запрос 1.7г через ANY.
SELECT DISTINCT r.route_id, r.destination, n.surname AS navigator
FROM flight AS f
JOIN navigator AS n USING (navigator_id)
JOIN route AS r USING (route_id)
WHERE n.navigator_id = ANY (
    SELECT sn.navigator_id
    FROM navigator AS sn
    WHERE sn.squadron = 'С.-Петербургский'
)
ORDER BY r.destination, r.route_id, navigator;

-- 2.2б. Все полёты с максимальным числом часов.
SELECT f.*
FROM flight AS f
WHERE f.flight_hours >= ALL (SELECT flight_hours FROM flight)
ORDER BY f.flight_id;

-- 2.2в. Пилоты с лимитом строго между минимумом и максимумом.
SELECT p.surname, p.hour_limit
FROM pilot AS p
WHERE p.hour_limit > ANY (SELECT hour_limit FROM pilot)
  AND p.hour_limit < ANY (SELECT hour_limit FROM pilot)
ORDER BY p.hour_limit, p.surname, p.pilot_id;

-- 2.2г. Рейсы с наименьшим коэффициентом сложности среди рейсов Святова.
WITH svyatov_routes AS (
    SELECT DISTINCT r.route_id, r.destination, r.difficulty_factor
    FROM flight AS f
    JOIN pilot AS p USING (pilot_id)
    JOIN route AS r USING (route_id)
    WHERE p.surname = 'Святов'
)
SELECT route_id, destination, difficulty_factor
FROM svyatov_routes
WHERE difficulty_factor <= ALL (SELECT difficulty_factor FROM svyatov_routes)
ORDER BY route_id;

-- 2.3. Авиаотряды пилотов или обслуживающего персонала рейсов.
SELECT squadron
FROM pilot
UNION
SELECT service_squadron
FROM route
ORDER BY squadron;

-- 2.4а. Пары пилот–штурман, работавшие на всех рейсах.
SELECT p.surname AS pilot, n.surname AS navigator
FROM (
    SELECT DISTINCT pilot_id, navigator_id FROM flight
) AS crew
JOIN pilot AS p USING (pilot_id)
JOIN navigator AS n USING (navigator_id)
WHERE NOT EXISTS (
    SELECT 1
    FROM route AS r
    WHERE NOT EXISTS (
        SELECT 1
        FROM flight AS f
        WHERE f.pilot_id = crew.pilot_id
          AND f.navigator_id = crew.navigator_id
          AND f.route_id = r.route_id
    )
)
ORDER BY pilot, navigator, crew.pilot_id, crew.navigator_id;

-- 2.4б. Рейсы только с нижегородскими пилотами и московскими штурманами.
SELECT r.route_id, r.destination
FROM route AS r
WHERE EXISTS (SELECT 1 FROM flight AS f WHERE f.route_id = r.route_id)
  AND NOT EXISTS (
      SELECT 1
      FROM flight AS f
      JOIN pilot AS p USING (pilot_id)
      JOIN navigator AS n USING (navigator_id)
      WHERE f.route_id = r.route_id
        AND (p.squadron <> 'Нижегородский' OR n.squadron !~ '^Московский[0-9]*$')
  )
ORDER BY r.route_id;

-- 2.4в. Штурманы, работавшие с нижегородскими пилотами на всех рейсах дольше 2 часов.
SELECT n.surname, n.squadron
FROM navigator AS n
WHERE EXISTS (SELECT 1 FROM route WHERE duration_hours > 2)
  AND NOT EXISTS (
      SELECT 1
      FROM route AS r
      WHERE r.duration_hours > 2
        AND NOT EXISTS (
            SELECT 1
            FROM flight AS f
            JOIN pilot AS p USING (pilot_id)
            WHERE f.route_id = r.route_id
              AND f.navigator_id = n.navigator_id
              AND p.squadron = 'Нижегородский'
        )
  )
ORDER BY n.surname, n.navigator_id;

-- 2.4г. Пилоты, у которых есть полёт по каждой требуемой паре рейс–штурман.
SELECT p.surname, p.squadron
FROM pilot AS p
WHERE EXISTS (SELECT 1 FROM route AS r WHERE r.service_squadron <> p.squadron)
  AND EXISTS (SELECT 1 FROM navigator WHERE hour_limit > 48)
  AND NOT EXISTS (
      SELECT 1
      FROM route AS r
      CROSS JOIN navigator AS n
      WHERE r.service_squadron <> p.squadron AND n.hour_limit > 48
        AND NOT EXISTS (
            SELECT 1
            FROM flight AS f
            WHERE f.pilot_id = p.pilot_id
              AND f.route_id = r.route_id
              AND f.navigator_id = n.navigator_id
        )
  )
ORDER BY p.surname, p.pilot_id;

-- 2.5а. Число различных авиаотрядов пилотов с полётами.
SELECT COUNT(DISTINCT p.squadron) AS squadron_count
FROM flight AS f
JOIN pilot AS p USING (pilot_id);

-- 2.5б. Средняя сложность разных рейсов пилотов отряда Московский1.
SELECT round(AVG(r.difficulty_factor), 3) AS average_difficulty
FROM route AS r
WHERE EXISTS (
    SELECT 1
    FROM flight AS f
    JOIN pilot AS p USING (pilot_id)
    WHERE f.route_id = r.route_id AND p.squadron = 'Московский1'
);

-- 2.5в. Суммарные часы по каждому пилоту, который работал на всех рейсах.
SELECT p.surname, SUM(f.flight_hours) AS total_hours
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
GROUP BY p.pilot_id
HAVING COUNT(DISTINCT f.route_id) = (SELECT COUNT(*) FROM route)
ORDER BY p.surname, p.pilot_id;

-- 2.5г. Число вылетов в Хабаровск у пилотов, летавших с Самариным более одного раза.
WITH qualified_pilots AS (
    SELECT f.pilot_id
    FROM flight AS f
    JOIN navigator AS n USING (navigator_id)
    WHERE n.surname = 'Самарин'
    GROUP BY f.pilot_id, n.navigator_id
    HAVING SUM(f.departure_count) > 1
)
SELECT p.surname,
       COALESCE(SUM(f.departure_count) FILTER (WHERE r.destination = 'Хабаровск'), 0)
           AS khabarovsk_departures
FROM pilot AS p
JOIN flight AS f USING (pilot_id)
JOIN route AS r USING (route_id)
WHERE p.pilot_id IN (SELECT pilot_id FROM qualified_pilots)
GROUP BY p.pilot_id
ORDER BY p.surname, p.pilot_id;

-- 2.6а. Месяцы и города, куда пилот совершил больше одного вылета.
SELECT p.surname,
       f.flight_month AS month,
       r.destination,
       SUM(f.departure_count) AS departures
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
JOIN route AS r USING (route_id)
GROUP BY p.pilot_id, month, r.destination
HAVING SUM(f.departure_count) > 1
ORDER BY p.surname, p.pilot_id, month, r.destination;

-- 2.6б. Список пилотов по каждому рейсу.
SELECT r.route_id, r.destination,
       string_agg(p.surname, ', ' ORDER BY p.surname, p.pilot_id) AS pilots
FROM route AS r
LEFT JOIN (SELECT DISTINCT route_id, pilot_id FROM flight) AS assigned USING (route_id)
LEFT JOIN pilot AS p USING (pilot_id)
GROUP BY r.route_id
ORDER BY r.route_id;

-- 2.6в. Месяцы с более чем тремя вылетами в Хабаровск.
SELECT f.flight_month AS month,
       SUM(f.departure_count) AS departures
FROM flight AS f
JOIN route AS r USING (route_id)
WHERE r.destination = 'Хабаровск'
GROUP BY month
HAVING SUM(f.departure_count) > 3
ORDER BY month;

-- 2.6г. Пилоты и месяцы с более чем 30 часами полётов.
SELECT p.surname,
       f.flight_month AS month,
       SUM(f.flight_hours) AS total_hours
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
GROUP BY p.pilot_id, month
HAVING SUM(f.flight_hours) > 30
ORDER BY p.surname, p.pilot_id, month;
