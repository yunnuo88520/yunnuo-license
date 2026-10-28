package store

import "context"

// Legacy archive deliberately exposes counts and product identity only. It
// never loads old ciphertext, shared secrets, signed tokens or card plaintext.
type AuthorizationArchiveItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Licenses  int64  `json:"licenses"`
	Cards     int64  `json:"cards"`
	Releases  int64  `json:"releases"`
}

func (s *Store) AuthorizationArchive(ctx context.Context) ([]AuthorizationArchiveItem, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT p.id,p.name,(SELECT COUNT(*) FROM licenses l WHERE l.product_id=p.id),(SELECT COUNT(*) FROM cards c WHERE c.product_id=p.id),(SELECT COUNT(*) FROM product_releases r WHERE r.product_id=p.id) FROM products p ORDER BY p.created_at,p.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []AuthorizationArchiveItem{}
	for rows.Next() {
		var item AuthorizationArchiveItem
		if e = rows.Scan(&item.ProductID, &item.Name, &item.Licenses, &item.Cards, &item.Releases); e != nil {
			return nil, e
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
