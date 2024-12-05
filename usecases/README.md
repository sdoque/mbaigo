# mbaigo use cases package


Package "usecases" addresses system behaviors and actions in given use cases such as configuration, registration, authentication, orchestration, ...

- Configuration: creates a new configuration file if it does not exist and later updates the system with deployment information.
- Authorization: creates a certificate signing request (CSR) to be sent to the CA.
- Registration: registers all services of a system with the lead Service Registrar and keeps it continuously updated.
- Servers & Requests: starts the severs for each protocol and directs requests.
- Provision: 