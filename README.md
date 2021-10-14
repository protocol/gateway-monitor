# gateway-monitor

Test the user experience for the IPFS gateways

Design doc: https://docs.google.com/document/d/1ozI4r8D8JNxdSoJqxty0JKuuzoujB0hSBkfsFZDOTL4/edit?usp=sharing


## Adding new tests

Each test is written in tasks/

Write your test there, and then add it to the `All` slice in tasks/tasks.go


## Running locally

If you have docker-compose, you can run a local instance.

```
docker-compose up -d
```

Then navigate your browser to the grafana instance at http://localhost:3000

*psst! password is ipfs*



# Bugs? Improvements?

PRs and issues welcome.
