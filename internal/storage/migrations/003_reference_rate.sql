CREATE TABLE IF NOT EXISTS user_rates (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    btc_rub_rate TEXT NOT NULL
);
