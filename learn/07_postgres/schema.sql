CREATE TABLE accounts (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    balance numeric NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
