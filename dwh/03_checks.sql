-- Проверки качества данных в oltp после загрузки.
-- Каждая проверка считает нарушения; в здоровых данных везде 0.
-- Если есть хоть одно нарушение, скрипт завершается с ошибкой (удобно для CI).
--
-- Запуск:
--   docker compose exec -T postgres psql -U vetcare -d vetcare -v ON_ERROR_STOP=1 < dwh/03_checks.sql

\set ON_ERROR_STOP on

CREATE TEMP TABLE checks AS

-- Полнота: ни одна строка не потерялась между raw и oltp.
SELECT 'строк в raw и oltp: ' || t AS check_name, abs(raw_rows - oltp_rows) AS problems
FROM (
    VALUES
        ('branches', (SELECT count(*) FROM raw.branches), (SELECT count(*) FROM oltp.branches)),
        ('doctors', (SELECT count(*) FROM raw.doctors), (SELECT count(*) FROM oltp.doctors)),
        ('owners', (SELECT count(*) FROM raw.owners), (SELECT count(*) FROM oltp.owners)),
        ('pets', (SELECT count(*) FROM raw.pets), (SELECT count(*) FROM oltp.pets)),
        ('medical_cards', (SELECT count(*) FROM raw.medical_cards), (SELECT count(*) FROM oltp.medical_cards)),
        ('vaccinations', (SELECT count(*) FROM raw.vaccinations), (SELECT count(*) FROM oltp.vaccinations)),
        ('visits', (SELECT count(*) FROM raw.visits), (SELECT count(*) FROM oltp.visits)),
        ('payments', (SELECT count(*) FROM raw.payments), (SELECT count(*) FROM oltp.payments))
) AS c (t, raw_rows, oltp_rows)

UNION ALL
SELECT 'данные загружены: есть приёмы', CASE WHEN EXISTS (SELECT 1 FROM oltp.visits) THEN 0 ELSE 1 END

-- Оплаты: ровно у состоявшихся приёмов.
UNION ALL
SELECT 'оплата у несостоявшегося приёма', count(*)
FROM oltp.payments p
JOIN oltp.visits v ON v.id = p.visit_id
WHERE v.status <> 'completed'

UNION ALL
SELECT 'состоявшийся приём без оплаты', count(*)
FROM oltp.visits v
WHERE v.status = 'completed'
  AND NOT EXISTS (SELECT 1 FROM oltp.payments p WHERE p.visit_id = v.id)

UNION ALL
SELECT 'оплата раньше начала приёма', count(*)
FROM oltp.payments p
JOIN oltp.visits v ON v.id = p.visit_id
WHERE p.paid_at < v.scheduled_at

-- Даты: приём не раньше найма врача и регистрации владельца.
UNION ALL
SELECT 'приём раньше найма врача', count(*)
FROM oltp.visits v
JOIN oltp.doctors d ON d.id = v.doctor_id
WHERE v.scheduled_at::date < d.hired_at

UNION ALL
SELECT 'приём раньше регистрации владельца', count(*)
FROM oltp.visits v
JOIN oltp.medical_cards mc ON mc.id = v.medical_card_id
JOIN oltp.pets pt ON pt.id = mc.pet_id
JOIN oltp.owners o ON o.id = pt.owner_id
WHERE v.scheduled_at::date < o.registered_at

UNION ALL
SELECT 'животное родилось после регистрации владельца', count(*)
FROM oltp.pets pt
JOIN oltp.owners o ON o.id = pt.owner_id
WHERE pt.birth_date >= o.registered_at

-- Связность.
UNION ALL
SELECT 'животное без медкарты', count(*)
FROM oltp.pets pt
WHERE NOT EXISTS (SELECT 1 FROM oltp.medical_cards mc WHERE mc.pet_id = pt.id)

UNION ALL
SELECT 'филиал приёма не совпадает с филиалом врача', count(*)
FROM oltp.visits v
JOIN oltp.doctors d ON d.id = v.doctor_id
WHERE v.branch_id <> d.branch_id

UNION ALL
SELECT 'хирургия не у хирурга', count(*)
FROM oltp.visits v
JOIN oltp.doctors d ON d.id = v.doctor_id
WHERE v.visit_type = 'surgery' AND d.specialization <> 'хирург'

UNION ALL
SELECT 'прививка без состоявшегося приёма-вакцинации', count(*)
FROM oltp.vaccinations vc
WHERE NOT EXISTS (
    SELECT 1
    FROM oltp.visits v
    JOIN oltp.medical_cards mc ON mc.id = v.medical_card_id
    WHERE mc.pet_id = vc.pet_id
      AND v.visit_type = 'vaccination'
      AND v.status = 'completed'
      AND v.scheduled_at::date = vc.vaccinated_at
)

-- Будущее: запланированные приёмы идут позже всех уже прошедших.
UNION ALL
SELECT 'запланированный приём раньше прошедших', count(*)
FROM oltp.visits v
WHERE v.status = 'scheduled'
  AND v.scheduled_at <= (SELECT max(scheduled_at) FROM oltp.visits WHERE status <> 'scheduled')
;

SELECT check_name AS "проверка", problems AS "нарушений",
       CASE WHEN problems = 0 THEN 'OK' ELSE 'ОШИБКА' END AS "итог"
FROM checks;

DO $$
DECLARE
    failed int;
BEGIN
    SELECT count(*) INTO failed FROM checks WHERE problems > 0;
    IF failed > 0 THEN
        RAISE EXCEPTION 'Проверок с нарушениями: %', failed;
    END IF;
    RAISE NOTICE 'Все проверки пройдены';
END
$$;
