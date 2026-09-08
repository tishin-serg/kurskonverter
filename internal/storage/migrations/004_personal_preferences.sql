CREATE TABLE IF NOT EXISTS user_preferences (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    min_orders INTEGER,
    min_success TEXT,
    allow_fallback INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS user_ui (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    state TEXT NOT NULL DEFAULT '',
    panel_id INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
