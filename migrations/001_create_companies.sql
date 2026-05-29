CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE company_type AS ENUM (
    'Corporations',
    'NonProfit',
    'Cooperative',
    'Sole Proprietorship'
);

CREATE TABLE IF NOT EXISTS companies (
    id               UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name             VARCHAR(15)  NOT NULL UNIQUE,
    description      VARCHAR(3000),
    amount_employees INT          NOT NULL CHECK (amount_employees >= 0),
    registered       BOOLEAN      NOT NULL DEFAULT FALSE,
    type             company_type NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
