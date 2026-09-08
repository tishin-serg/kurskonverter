CREATE TABLE provider_payments (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider TEXT NOT NULL,
 payment_method TEXT NOT NULL,
 PRIMARY KEY(user_id,provider)
);
