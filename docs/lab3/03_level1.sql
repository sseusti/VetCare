-- 1.3. Проверка содержимого всех четырёх таблиц.
SELECT * FROM pilot ORDER BY pilot_id;
SELECT * FROM navigator ORDER BY navigator_id;
SELECT * FROM route ORDER BY route_id;
SELECT * FROM flight ORDER BY flight_id;

-- 1.4а. Различные авиаотряды пилотов.
SELECT DISTINCT squadron
FROM pilot
ORDER BY squadron;

-- 1.4б. Различные пункты назначения.
SELECT DISTINCT destination
FROM route
ORDER BY destination;

-- 1.4в. Различные лимиты часов штурманов.
SELECT DISTINCT hour_limit
FROM navigator
ORDER BY hour_limit;

-- 1.5а. Пилоты с лимитом больше 30 часов.
SELECT surname, squadron
FROM pilot
WHERE hour_limit > 30
ORDER BY surname, pilot_id;

-- 1.5б. Пункты назначения рейсов с нижегородским обслуживанием.
SELECT DISTINCT destination
FROM route
WHERE service_squadron = 'Нижегородский'
ORDER BY destination;

-- 1.5в. Записи о полётах продолжительностью больше 8 часов.
SELECT f.flight_id, f.flight_month, p.surname AS pilot
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
WHERE f.flight_hours > 8
ORDER BY f.flight_id;

-- 1.6а. Номер записи, пилот и часы по возрастанию.
SELECT f.flight_id, p.surname AS pilot, f.flight_hours
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
ORDER BY f.flight_hours, f.flight_id;

-- 1.6б. Пункт назначения, вылеты, штурман и месяц.
SELECT r.destination, f.departure_count, n.surname AS navigator, f.flight_month
FROM flight AS f
JOIN route AS r USING (route_id)
JOIN navigator AS n USING (navigator_id)
ORDER BY f.flight_month, f.flight_id;

-- 1.7а. Штурманы, летавшие в Хабаровск или с пилотом своего авиаотряда.
SELECT DISTINCT n.surname
FROM flight AS f
JOIN route AS r USING (route_id)
JOIN navigator AS n USING (navigator_id)
JOIN pilot AS p USING (pilot_id)
WHERE r.destination = 'Хабаровск' OR n.squadron = p.squadron
ORDER BY n.surname;

-- 1.7б. Пилоты на рейсах дольше трёх часов с февраля включительно.
SELECT DISTINCT p.surname, p.hour_limit
FROM flight AS f
JOIN route AS r USING (route_id)
JOIN pilot AS p USING (pilot_id)
WHERE r.duration_hours > 3 AND f.flight_month >= 2
ORDER BY p.surname, p.hour_limit;

-- 1.7в. Штурманы, совершившие с Поливцевым больше одного вылета.
SELECT n.surname, n.squadron
FROM flight AS f
JOIN pilot AS p USING (pilot_id)
JOIN navigator AS n USING (navigator_id)
WHERE p.surname = 'Поливцев'
GROUP BY p.pilot_id, n.navigator_id
HAVING SUM(f.departure_count) > 1
ORDER BY n.surname, n.navigator_id;

-- 1.7г. Рейсы и штурманы С.-Петербургского авиаотряда.
SELECT DISTINCT r.route_id, r.destination, n.surname AS navigator
FROM flight AS f
JOIN navigator AS n USING (navigator_id)
JOIN route AS r USING (route_id)
WHERE n.squadron = 'С.-Петербургский'
ORDER BY r.destination, r.route_id, navigator;

-- 1.8. Пересчёт часов по коэффициенту сложности рейса.
UPDATE flight AS f
SET flight_hours = f.flight_hours * r.difficulty_factor
FROM route AS r
WHERE r.route_id = f.route_id;

SELECT flight_id, flight_hours
FROM flight
ORDER BY flight_id;

-- 1.9. Доля скорректированных часов от лимита пилота, в процентах.
ALTER TABLE flight ADD COLUMN hours_limit_pct numeric(20, 2);

UPDATE flight AS f
SET hours_limit_pct = round(100 * f.flight_hours / p.hour_limit, 2)
FROM pilot AS p
WHERE p.pilot_id = f.pilot_id;

ALTER TABLE flight
    ALTER COLUMN hours_limit_pct SET NOT NULL,
    ADD CHECK (hours_limit_pct >= 0);

SELECT flight_id, flight_hours, hours_limit_pct
FROM flight
ORDER BY flight_id;
