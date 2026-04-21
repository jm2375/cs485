# Frontend-to-Backend Integration Test Suite

Requires: `BASE_URL` env variable pointing at a running backend.  
All tests are skipped when `BASE_URL` is unset (safe for unit-test CI).

**Run locally:**
```
BASE_URL=https://your-api.example.com npm run test:integration
```

**Run in CI:** set the `BASE_URL` / `BACKEND_BASE_URL` GitHub secret.

## Covered Endpoints

| ID    | Method | Path                                         | Auth         | Request Body / Query Params           | 2xx | Key Error Cases                           |
|-------|--------|----------------------------------------------|--------------|---------------------------------------|-----|-------------------------------------------|
| H-01  | GET    | /health                                      | No           | —                                     | 200 | —                                         |
| A-01  | POST   | /api/auth/register                           | No           | {email, displayName, password}        | 201 | 409 duplicate email                       |
| A-02  | POST   | /api/auth/login                              | No           | {email, password}                     | 200 | 401 wrong password                        |
| A-03  | GET    | /api/auth/me                                 | Bearer token | —                                     | 200 | 401 no / bad token                        |
| A-04  | POST   | /api/dev/bootstrap                           | No           | —                                     | 200 | —                                         |
| P-01  | GET    | /api/pois/search                             | No           | ?q=&category=&near=                   | 200 | —                                         |
| P-02  | GET    | /api/pois/search?category=restaurant         | No           | ?category=restaurant                  | 200 | —                                         |
| P-03  | GET    | /api/pois/search?q=\<text\>                  | No           | ?q=ramen                              | 200 | —                                         |
| P-04  | GET    | /api/pois/search?q=\<no-match\>              | No           | ?q=zzznomatch999                      | 200 | returns empty pois array                  |
| T-01  | GET    | /api/trips                                   | Bearer token | —                                     | 200 | 401 no token                              |
| T-02  | POST   | /api/trips                                   | Bearer token | {name, destination}                   | 201 | 401 no token                              |
| T-03  | GET    | /api/trips/:tripId                           | Bearer token | —                                     | 200 | 401 no token, 403 non-member              |
| T-04  | PATCH  | /api/trips/:tripId                           | Bearer token | {name?, destination?}                 | 200 | 401, 403 non-member                       |
| T-05  | GET    | /api/trips/:tripId/share-link                | Bearer token | —                                     | 200 | 401, 403 non-member                       |
| T-06  | POST   | /api/trips/:tripId/share-link/regenerate     | Bearer token | —                                     | 200 | 401, 403 non-owner                        |
| SL-01 | GET    | /api/join/:inviteCode                        | No           | —                                     | 200 | 404 bad code                              |
| SL-02 | POST   | /api/join/:inviteCode                        | Bearer token | —                                     | 200 | 401 no token, 404 bad code                |
| C-01  | GET    | /api/trips/:tripId/collaborators             | Bearer token | —                                     | 200 | 401, 403 non-member                       |
| C-02  | PATCH  | /api/trips/:tripId/collaborators/:userId     | Bearer token | {role}                                | 200 | 401, 403 non-owner                        |
| C-03  | DELETE | /api/trips/:tripId/collaborators/:userId     | Bearer token | —                                     | 204 | 400 remove owner, 403, 404                |
| I-01  | POST   | /api/trips/:tripId/invitations               | Bearer token | {emails, role}                        | 201 | 403 viewer, 422 all-dups                  |
| I-02  | GET    | /api/trips/:tripId/invitations               | Bearer token | ?status=                              | 200 | 401, 403 non-member                       |
| I-03  | GET    | /api/invitations/accept/:token               | No           | —                                     | 200 | 404 bad token                             |
| I-04  | POST   | /api/invitations/accept/:token               | Bearer token | —                                     | 200 | 401 no token, 404 bad token               |
| I-05  | DELETE | /api/invitations/:id                         | Bearer token | —                                     | 204 | 401, 403 non-owner, 404                   |
| IT-01 | GET    | /api/trips/:tripId/itinerary                 | Bearer token | —                                     | 200 | 401, 403 non-member                       |
| IT-02 | POST   | /api/trips/:tripId/itinerary                 | Bearer token | {poiId, day, notes?}                  | 201 | 403 viewer, 409 duplicate POI             |
| IT-03 | DELETE | /api/trips/:tripId/itinerary/:itemId         | Bearer token | —                                     | 204 | 403 viewer, 404 not found                 |
