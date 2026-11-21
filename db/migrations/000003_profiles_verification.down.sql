DROP TABLE verification_submissions;
ALTER TABLE organizations DROP COLUMN public_profile, DROP COLUMN private_profile;
ALTER TABLE users DROP COLUMN personal_profile, DROP COLUMN personal_verification_status;
