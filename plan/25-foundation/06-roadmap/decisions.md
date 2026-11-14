# Decision records (ui-core build)

| id | surface | question | options | taken | why | door | fixture |
|---|---|---|---|---|---|---|---|
| D-01 | dock/phone, inside | Fight inside a building hides Leave | leave surface on the row; refuse engage inside | leave surface | Nico keeps a way out; Mo calm | two-way | fight-inside@390 |
| D-02 | menu/account | Where does a player add a passkey, an authenticator app, recovery codes or change the password? (authentik refuses `/if/user/` to external users; its setup flows work for anyone signed in) | Account panel in the Menu; per-app links; authentik user page | Account panel in the Menu (S4) | one entry for every ukubi app on the core; external visitors keep account safety; links out to the stock authentik flows with `?next=` back, no authentik API | two-way | account@390 |
