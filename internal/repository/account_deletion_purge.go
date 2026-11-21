package repository

import (
	"context"
	"database/sql"
	"fmt"
)

type deletionStatement struct {
	query string
	uses  int
}

var deleteAccountTrackData = []deletionStatement{
	{`DELETE i FROM track_submission_images i JOIN track_submissions s ON s.submission_id=i.submission_id WHERE s.user_id=?`, 1},
	{`DELETE e FROM track_submission_events e JOIN track_submissions s ON s.submission_id=e.submission_id WHERE s.user_id=?`, 1},
	{`DELETE FROM track_submissions WHERE user_id=?`, 1},
	{`DELETE w FROM track_waypoints w LEFT JOIN track_records t ON t.id=w.track_id WHERE w.user_id=? OR t.user_id=?`, 2},
	{`DELETE c FROM track_collects c LEFT JOIN track_records t ON t.id=c.track_id WHERE c.user_id=? OR t.user_id=?`, 2},
	{`DELETE n FROM track_navigations n LEFT JOIN track_records t ON t.id=n.track_id WHERE n.navigator_user_id=? OR t.user_id=?`, 2},
	{`DELETE j FROM track_map_index_jobs j JOIN track_records t ON t.id=j.track_id WHERE t.user_id=?`, 1},
	{`DELETE FROM track_geo_indexes WHERE user_id=?`, 1},
	{`DELETE l FROM track_scenic_spot_links l JOIN track_records t ON t.id=l.track_id WHERE t.user_id=?`, 1},
	{`DELETE p FROM track_place_records p JOIN track_records t ON p.record_key=CONCAT('match:',t.id) WHERE t.user_id=?`, 1},
	{`DELETE l FROM track_likes l LEFT JOIN track_records t ON t.id=l.track_id WHERE l.user_id=? OR t.user_id=?`, 2},
	{`DELETE s FROM recommend_item_stats_daily s JOIN track_records t ON t.id=s.track_id WHERE t.user_id=?`, 1},
	{`DELETE a FROM ai_usage_tasks a JOIN track_records t ON t.id=a.object_id WHERE t.user_id=?`, 1},
	{`DELETE d FROM track_route_introductions d JOIN track_records t ON t.id=d.anchor_track_id WHERE t.user_id=?`, 1},
	{`UPDATE track_route_groups g
		JOIN (SELECT DISTINCT m.group_id FROM track_route_group_members m JOIN track_records t ON t.id=m.track_id WHERE t.user_id=?) changed ON changed.group_id=g.group_id
		LEFT JOIN track_records rep ON rep.id=g.representative_track_id
		LEFT JOIN (SELECT m.group_id,COUNT(*) member_count,MIN(m.track_id) replacement FROM track_route_group_members m
		 JOIN track_records t ON t.id=m.track_id WHERE t.user_id<>? GROUP BY m.group_id) keepers ON keepers.group_id=g.group_id
		SET g.member_count=COALESCE(keepers.member_count,0),
		 g.representative_track_id=CASE WHEN rep.user_id=? THEN COALESCE(keepers.replacement,'') ELSE g.representative_track_id END,
		 g.status=CASE WHEN keepers.member_count IS NULL THEN 'archived' ELSE g.status END, g.updated_at=UTC_TIMESTAMP(6)`, 3},
	{`UPDATE track_route_group_members m JOIN track_route_groups g ON g.group_id=m.group_id
		JOIN (SELECT DISTINCT doomed.group_id FROM track_route_group_members doomed JOIN track_records t ON t.id=doomed.track_id WHERE t.user_id=?) changed ON changed.group_id=m.group_id
		SET m.role=CASE WHEN m.track_id=g.representative_track_id THEN 'representative' ELSE 'member' END`, 1},
	{`DELETE m FROM track_route_group_members m JOIN track_records t ON t.id=m.track_id WHERE t.user_id=?`, 1},
}

var deleteAccountCommentData = []deletionStatement{
	{`UPDATE track_comments c JOIN track_comment_reactions r ON r.comment_id=c.id
		SET c.like_count=GREATEST(0,c.like_count-(r.reaction=1)), c.dislike_count=GREATEST(0,c.dislike_count-(r.reaction=-1))
		WHERE r.user_id=?`, 1},
	{`DELETE r FROM track_comment_reactions r JOIN track_comments c ON c.id=r.comment_id
		LEFT JOIN track_records t ON t.id=c.track_id WHERE r.user_id=? OR c.user_id=? OR t.user_id=?`, 3},
	{`DELETE i FROM track_comment_images i LEFT JOIN track_records t ON t.id=i.track_id WHERE i.user_id=? OR t.user_id=?`, 2},
	{`UPDATE track_comment_stats s JOIN (SELECT track_id,COUNT(*) removed FROM track_comments WHERE user_id=? AND deleted=0 GROUP BY track_id) c ON c.track_id=s.track_id
		SET s.total_count=GREATEST(0,s.total_count-c.removed)`, 1},
	{`UPDATE track_comments root JOIN (SELECT root_id,COUNT(*) removed FROM track_comments WHERE user_id=? AND deleted=0 AND root_id<>'' GROUP BY root_id) c ON c.root_id=root.id
		SET root.reply_count=GREATEST(0,root.reply_count-c.removed)`, 1},
	{`DELETE s FROM track_comment_stats s JOIN track_records t ON t.id=s.track_id WHERE t.user_id=?`, 1},
	{`DELETE c FROM track_comments c JOIN track_records t ON t.id=c.track_id WHERE t.user_id=?`, 1},
	{`UPDATE track_comments SET user_id=0, client_id=id, request_hash='', content='', media_json='[]', deleted=1,
		like_count=0,dislike_count=0,updated_at=UTC_TIMESTAMP(6) WHERE user_id=?`, 1},
}

var deleteAccountCompanionData = []deletionStatement{
	{`DELETE FROM companion_track_chunks WHERE user_id=?`, 1},
	{`DELETE FROM companion_live_positions WHERE user_id=?`, 1},
	{`DELETE FROM companion_danmakus WHERE user_id=?`, 1},
	{`DELETE FROM companion_events WHERE owner_user_id=? OR target_user_id=?`, 2},
	{`UPDATE companion_history_removals SET actor_user_id=0 WHERE actor_user_id=?`, 1},
	{`DELETE FROM companion_history_removals WHERE user_id=?`, 1},
	{`DELETE FROM companion_session_members WHERE user_id=?`, 1},
	{`DELETE s FROM companion_sessions s LEFT JOIN companion_session_members m ON m.session_id=s.session_id
		WHERE s.owner_user_id=? AND m.user_id IS NULL`, 1},
	{`UPDATE companion_sessions s
		JOIN (SELECT session_id,MIN(user_id) new_owner,COUNT(*) participants FROM companion_session_members GROUP BY session_id) members ON members.session_id=s.session_id
		JOIN companion_session_members m ON m.session_id=s.session_id AND m.user_id=members.new_owner
		SET s.owner_user_id=members.new_owner,m.role='owner',s.status='ended',s.visibility='private',s.ended_at=COALESCE(s.ended_at,UTC_TIMESTAMP(6)),
		 s.end_operator_user_id=0,s.actual_participant_count=members.participants,s.title='',s.locate_addr='',
		 s.total_distance=0,s.total_duration=0,s.cover_preview=NULL,s.track_screenshot_url='',s.updated_at=UTC_TIMESTAMP(6)
		WHERE s.owner_user_id=?`, 1},
	{`UPDATE companion_sessions SET end_operator_user_id=0 WHERE end_operator_user_id=?`, 1},
}

var deleteAccountPersonalData = []deletionStatement{
	{`DELETE FROM track_records WHERE user_id=?`, 1},
	{`DELETE FROM user_follows WHERE follower_user_id=? OR followee_user_id=?`, 2},
	{`DELETE FROM user_activity_days WHERE user_id=?`, 1},
	{`DELETE FROM user_account_restrictions WHERE user_id=?`, 1},
	{`DELETE FROM user_achievement_rewards WHERE user_id=?`, 1},
	{`DELETE FROM user_permission_snapshots WHERE user_id=?`, 1},
	{`DELETE FROM login_log WHERE user_id=?`, 1},
	{`DELETE FROM app_client_devices WHERE user_id=?`, 1},
	{`DELETE FROM user_feedbacks WHERE user_id=?`, 1},
	{`DELETE FROM message_records WHERE user_id=? OR peer_id=?`, 2},
	{`DELETE FROM recommend_feed_sessions WHERE user_id=?`, 1},
	{`DELETE FROM recommend_user_profiles WHERE user_id=?`, 1},
	{`UPDATE app_releases SET operator_user_id=0 WHERE operator_user_id=?`, 1},
	{`DELETE FROM users WHERE id=?`, 1},
}

func purgeAccountData(ctx context.Context, tx *sql.Tx, userID int64) error {
	groups := [][]deletionStatement{deleteAccountTrackData, deleteAccountCommentData, deleteAccountCompanionData, deleteAccountPersonalData}
	for groupIndex, group := range groups {
		for index, step := range group {
			args := make([]any, step.uses)
			for i := range args {
				args[i] = userID
			}
			if _, err := tx.ExecContext(ctx, step.query, args...); err != nil {
				return fmt.Errorf("account cleanup group %d statement %d: %w", groupIndex+1, index+1, err)
			}
		}
	}
	return nil
}
