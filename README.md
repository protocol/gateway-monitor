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

Then navigate your browser to the grafana instance at http://localhost:3000/d/n1qvDWO7k/gateway-monitor-tasks?orgId=1

*psst! password is ipfs*

## Deployment

Production deployment is done by CircleCI when merging to `master`. Be sure to keep
an eye on the build, since there seems to be a race condition that sometimes causes
deployment to error out for some region(s).

# Bugs? Improvements?

PRs and issues welcome.
