DROP SCHEMA IF EXISTS oltp CASCADE;
CREATE SCHEMA oltp;

CREATE TABLE oltp.branches (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    address TEXT NOT NULL
);

CREATE TABLE oltp.doctors (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    last_name TEXT NOT NULL,
    first_name TEXT NOT NULL,
    middle_name TEXT,
    phone TEXT UNIQUE NOT NULL,
    specialization TEXT NOT NULL,
    branch_id BIGINT NOT NULL REFERENCES oltp.branches (id) ON DELETE RESTRICT,
    hired_at DATE NOT NULL
);

CREATE TABLE oltp.owners (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    last_name TEXT NOT NULL,
    first_name TEXT NOT NULL,
    middle_name TEXT,
    phone TEXT UNIQUE NOT NULL,
    registered_at DATE NOT NULL
);

CREATE TABLE oltp.pets (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES oltp.owners (id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    species TEXT NOT NULL,
    breed TEXT NOT NULL,
    color TEXT NOT NULL,
    sex TEXT NOT NULL CHECK (sex IN ('M', 'F')),
    size TEXT NOT NULL CHECK (size IN ('small', 'medium', 'large')),
    birth_date DATE NOT NULL
);

CREATE TABLE oltp.medical_cards (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pet_id BIGINT NOT NULL UNIQUE REFERENCES oltp.pets (id) ON DELETE RESTRICT,
    chronic_diseases TEXT,
    allergies TEXT,
    created_at DATE NOT NULL
);

CREATE TABLE oltp.vaccinations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pet_id BIGINT NOT NULL REFERENCES oltp.pets (id) ON DELETE RESTRICT,
    vaccine TEXT NOT NULL,
    vaccinated_at DATE NOT NULL
);

CREATE TABLE oltp.visits (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    medical_card_id BIGINT NOT NULL REFERENCES oltp.medical_cards (id) ON DELETE RESTRICT,
    doctor_id BIGINT NOT NULL REFERENCES oltp.doctors (id) ON DELETE RESTRICT,
    branch_id BIGINT NOT NULL REFERENCES oltp.branches (id) ON DELETE RESTRICT,
    cabinet TEXT NOT NULL,
    booked_at TIMESTAMPTZ NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    visit_type TEXT NOT NULL CHECK (visit_type IN ('checkup', 'vaccination', 'treatment', 'surgery')),
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show')),
    diagnosis TEXT,
    recommendations TEXT,
    reminder_sent BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT visits_booked_before_scheduled CHECK (booked_at <= scheduled_at),
    CONSTRAINT visits_diagnosis_only_if_completed CHECK (status = 'completed' OR diagnosis IS NULL)
);

CREATE TABLE oltp.payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    visit_id BIGINT NOT NULL UNIQUE REFERENCES oltp.visits (id) ON DELETE RESTRICT,
    service TEXT NOT NULL,
    amount NUMERIC(10, 2) NOT NULL CHECK (amount >= 0),
    discount NUMERIC(5, 2) NOT NULL DEFAULT 0 CHECK (discount BETWEEN 0 AND 100),
    payment_method TEXT NOT NULL CHECK (payment_method IN ('cash', 'card', 'online')),
    paid_at TIMESTAMPTZ NOT NULL
);

COMMENT ON COLUMN oltp.payments.amount IS 'Цена услуги до скидки, руб.';
COMMENT ON COLUMN oltp.payments.discount IS 'Скидка в процентах; оплачено = amount * (1 - discount / 100)';
COMMENT ON COLUMN oltp.visits.reminder_sent IS 'Было ли отправлено SMS-напоминание (для A/B-анализа неявок)';

CREATE INDEX ON oltp.doctors (branch_id);
CREATE INDEX ON oltp.pets (owner_id);
CREATE INDEX ON oltp.vaccinations (pet_id);
CREATE INDEX ON oltp.visits (medical_card_id);
CREATE INDEX ON oltp.visits (doctor_id);
CREATE INDEX ON oltp.visits (branch_id);
CREATE INDEX ON oltp.visits (scheduled_at);
