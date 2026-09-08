package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Preferences struct {
	MinOrders     *int
	MinSuccess    string
	AllowFallback bool
}

func (s *Store) Preferences(ctx context.Context, user int64) (Preferences, error) {
	p := Preferences{AllowFallback: true}
	var orders sql.NullInt64
	var success sql.NullString
	err := s.DB.QueryRowContext(ctx, "SELECT min_orders,min_success,allow_fallback FROM user_preferences WHERE user_id=?", user).Scan(&orders, &success, &p.AllowFallback)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if orders.Valid {
		n := int(orders.Int64)
		p.MinOrders = &n
	}
	p.MinSuccess = success.String
	return p, err
}
func (s *Store) SetPreference(ctx context.Context, user int64, key, value string) error {
	column := map[string]string{"orders": "min_orders", "success": "min_success", "fallback": "allow_fallback"}[key]
	if column == "" {
		return fmt.Errorf("unknown preference")
	}
	_, err := s.DB.ExecContext(ctx, "INSERT INTO user_preferences(user_id,"+column+") VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET "+column+"=excluded."+column, user, value)
	return err
}

type UIState struct {
	State     string
	PanelID   int
	Revision  int64
	UpdatedAt int64
}

func (s *Store) UIState(ctx context.Context, user int64) (UIState, error) {
	var v UIState
	err := s.DB.QueryRowContext(ctx, "SELECT state,panel_id,revision,updated_at FROM user_ui WHERE user_id=?", user).Scan(&v.State, &v.PanelID, &v.Revision, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return v, nil
	}
	return v, err
}
func (s *Store) SaveUI(ctx context.Context, user int64, v UIState) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO user_ui(user_id,state,panel_id,revision,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET state=excluded.state,panel_id=excluded.panel_id,revision=excluded.revision,updated_at=excluded.updated_at", user, v.State, v.PanelID, v.Revision, time.Now().Unix())
	return err
}
