# mbaigo use cases package


Package "usecases" addresses system behaviors and actions in given use cases such as configuration, registration, authentication, orchestration, ...

- authentication: creates a private key and a certificate signing request (CSR) to be sent to the CA. It is not yet going authentication, just "certification"
- configuration: creates a new configuration file if it does not exist and later updates the system with deployment information
- consumption: makes requests to providers, and invokes serviceDiscovery if the service URL is unknown
- cost: handles service costs
- kgaphing: generates the knowledge graph of the system
- packing: marshals and un-marshals forms which all communications use
- provision: responding so consumers'requests
- registration: registers all services of a system with the lead Service Registrar and keeps it continuously updated.
- service discovery: interaction with the Orchestrator to obtain all service URL based on description
- subscription: based on the Observer pattern, keep track of subscribers