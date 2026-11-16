-- Group-bot operation has no anonymous/player self-service distinction.
-- Every named participant is a normal player with retained rating history.
ALTER TABLE players DROP COLUMN is_guest;
