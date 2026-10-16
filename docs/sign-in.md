# Sign-in and passwords

## Shortest password allowed

Settings > Sign-in > Passwords sets the shortest password HolyArrs takes. Only the owner can change it.

- Default: 4 characters. Allowed: 4 to 64. HolyArrs never goes below 4, whatever the setting says.
- It counts for every new password: the first-run owner, an invite being accepted, a password change on My account, and `holyarrs reset-owner`.
- Passwords people already have keep working after the minimum goes up; the new minimum applies the next time they change it.
- The sign-in, invite and My account pages read the minimum from the server (`min_password_length` in `GET /api/v1/auth/state`), so the hint under the password box always matches.
- The longest password taken is 1024 characters.

Short passwords are easy to guess. HolyArrs slows down repeated wrong sign-ins from the same place and for the same name, which helps, but it does not make a short password safe. When HolyArrs can be reached from outside your home, raise the minimum to 12 or more.

API: `GET /api/v1/auth/settings` shows `min_password_length`; `PUT /api/v1/auth/settings` with `{"min_password_length": 12}` changes it (owner only, 4 to 64, anything else answers 400).
