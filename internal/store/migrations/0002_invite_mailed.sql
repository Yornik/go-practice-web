-- When the site mailed an invite through SMTP. The mail holds the only
-- copy of its link, so opening it proves control of the address, and a
-- sign-up with that address is verified at once. NULL for an invite
-- without an address, whose mail failed, or that only reached the log
-- because SMTP is not configured.
ALTER TABLE invites ADD COLUMN mailed_at TEXT;
