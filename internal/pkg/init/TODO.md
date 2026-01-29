# Init Wizard - TODO

## Planned Improvements

### Multi-type Project Selection
- [ ] Allow selecting multiple project types (e.g., REST + Telegram Bot)
- Currently only one type can be selected
- Service may include multiple transports: REST API, gRPC, Telegram bot, Kafka consumers
- Need to change from single Select to MultiSelect in `askProjectType()`
- Combine configurations from multiple types into single project.yaml

### Other Ideas
- [ ] Add validation for project name (no spaces, valid Go package name)
- [ ] Add option to import existing OpenAPI spec for REST projects
- [ ] Add option to import existing .proto files for gRPC projects
- [ ] Interactive driver configuration (databases, caches, etc.)
