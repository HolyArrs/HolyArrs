package subtitles

import "context"

// cleanIDs lists the SRT subtitles HolyArrs saved beside the videos of one
// File, or of every File when fileID is 0.
func (s *Service) cleanIDs(ctx context.Context, fileID int64) ([]int64, error) {
	q := "SELECT id FROM subtitles WHERE place = 'beside' AND state = 'present' AND relative_path LIKE '%.srt' AND origin IN ('fetched', 'by_hand')"
	var args []any
	if fileID != 0 {
		q += " AND file_id = ?"
		args = append(args, fileID)
	}
	rows, err := s.Store.Read().QueryContext(ctx, q+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
