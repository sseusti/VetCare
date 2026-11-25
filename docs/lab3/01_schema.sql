-- 1.1. Создание схемы и таблиц варианта 15.
CREATE SCHEMA lab3_v15;
SET search_path TO lab3_v15;

CREATE TABLE pilot (
    pilot_id integer PRIMARY KEY,
    surname text NOT NULL CHECK (btrim(surname) <> ''),
    squadron text NOT NULL CHECK (btrim(squadron) <> ''),
    hour_limit numeric(7, 2) NOT NULL CHECK (hour_limit > 0)
);

CREATE TABLE navigator (
    navigator_id integer PRIMARY KEY,
    surname text NOT NULL CHECK (btrim(surname) <> ''),
    squadron text NOT NULL CHECK (btrim(squadron) <> ''),
    hour_limit numeric(7, 2) NOT NULL CHECK (hour_limit > 0)
);

CREATE TABLE route (
    route_id integer PRIMARY KEY,
    destination text NOT NULL CHECK (btrim(destination) <> ''),
    service_squadron text NOT NULL CHECK (btrim(service_squadron) <> ''),
    duration_hours numeric(7, 2) NOT NULL CHECK (duration_hours > 0),
    difficulty_factor numeric(6, 3) NOT NULL CHECK (difficulty_factor > 0)
);

CREATE TABLE flight (
    flight_id integer PRIMARY KEY,
    flight_month smallint NOT NULL CHECK (flight_month BETWEEN 1 AND 12),
    pilot_id integer NOT NULL REFERENCES pilot (pilot_id),
    navigator_id integer NOT NULL REFERENCES navigator (navigator_id),
    route_id integer NOT NULL REFERENCES route (route_id),
    departure_count integer NOT NULL CHECK (departure_count > 0),
    flight_hours numeric(14, 5) NOT NULL CHECK (flight_hours > 0)
);
