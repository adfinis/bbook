# bbook

bbook is the internal Adfinis contact address book. It fetches customer contacts from Zoho CRM and makes them available in two ways: a searchable web page, and a read-only CardDAV address bbook that can be added to mail clients.

## Components

The application is a single Go binary that runs an HTTP server. Around it:

* **PostgreSQL** stores the contacts. The schema is managed through migrations applied on startup.
* **Zoho CRM** is the source of truth. A background job fetches the contacts at a fixed interval and upserts them into the database.
* **OIDC** handles login for the web interface. In the docker-compose there is a preconfigured Keycloak for development.
* **Search** runs in memory. The index is rebuilt from the database on startup and queried by the web interface and the CardDAV endpoint.
* **CardDAV** is served under `/api/dav`. Mail clients authenticate with per-client tokens that users generate from the UI.

## Development

```sh
cd docker
docker compose up
```

Configuration is read from the `.env`, `.db.env` and `.backend.env` files in `docker/`.

## CI and releases

To create a new release, merge to the `release` branch.