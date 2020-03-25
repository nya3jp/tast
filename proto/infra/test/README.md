This directory contains the API definitions for test metadata and execution a la
[go/cros-f20](http://go/cros-f20).

This API is currently at *alpha* stability level (https://aip.dev/181)

* API definitions may be updated frequently. Changes must not break binary
  compatibility, but may be semantically breaking. Existing fields may be
  deprecated and then removed after a short waiting period.
* All users must be whitelisted to depend on this API and be available for easy
  communication of any semantically breaking changes. If you depend on this API
  you should subscribe to [g/cros-f20-discuss](http://g/cros-f20-discuss)

## Directory structure

The following high level directory structure is intended to aid understanding
the API:

* [metadata/v1/](metadata/v1/) defines the schema used to generate test metadata
  used for scheduling, execution and analytics.
* [rtd/v1/](rtd/v1/]) defines a generic API used to interact with Remote Test
  Drivers for test execution.
* [plan/v1/](plan/v1/) defines the schema used to specify high-level test plans
  intended to capture the business needs of Chrome OS developers from hardware
  testing.