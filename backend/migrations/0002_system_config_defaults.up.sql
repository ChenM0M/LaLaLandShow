INSERT INTO system_config (key, value_json, updated_by)
VALUES
  (
    'booking_open_at',
    to_jsonb(to_char((now() - interval '1 hour') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')),
    'system'
  ),
  (
    'booking_lock_at',
    to_jsonb(to_char((now() + interval '30 days') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')),
    'system'
  ),
  (
    'show_start_at',
    to_jsonb(to_char((now() + interval '31 days') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')),
    'system'
  ),
  ('play_name', to_jsonb('La La Land Graduation Play'::text), 'system'),
  ('venue', to_jsonb('School Auditorium'::text), 'system')
ON CONFLICT (key) DO NOTHING;
