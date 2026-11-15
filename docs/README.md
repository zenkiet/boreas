# Boreas documentation

Boreas gives each development task its own test environment and link. Teams
can test several changes at once without replacing each other's builds on a
shared staging server.

For example, a checkout fix named `wo-718` in the `shop` project runs at
`https://boreas.example.com/shop/wo-718/`. Developers deploy updates to that
same link; testers check it when the task is marked ready.

## The basic idea

| Term | Meaning |
| ---- | ------- |
| Project | A group of related tasks, usually for one product or repository |
| Task | One test environment, running one Docker container |
| Image | The packaged app that the container runs; your pipeline builds it |
| App URL | The task's link: `/{project}/{task}/` |
| Dev status | Whether people consider the work in progress, blocked or ready for testing |

Boreas runs and tracks environments. It does not build your app or automatically
create an environment for every branch. Your team or pipeline creates tasks and
deploys images to them.

**Access matters:** the directory and App URLs are public to anyone who can
reach the server. Management requires an account. Project roles do not make
App URLs private; use task passwords and network restrictions where needed.

## Choose a guide

| You want to… | Read |
| ------------ | ---- |
| Install Boreas and open a first task | [Get started](getting-started.md) |
| Understand statuses or change an environment | [Work with tasks](tasks.md) |
| Add people and choose their permissions | [Projects and people](projects-and-people.md) |
| Deploy builds automatically | [Deploy from your pipeline](deploy-from-ci.md) |
| Ask about logs, builds or code; plan a ticket | [Bora](bora.md) |
| Receive alerts in the app, chat or on a phone | [Notifications](notifications.md) |
| Configure the server and optional services | [Configuration](configuration.md) |
| Integrate another app or script | [API](api.md) |
| Change Boreas itself | [Development](development.md) |

## About the examples

The server includes a public directory, not the separate web/iOS/Android team
app shown in the [README screenshots](../README.md#screenshots). These guides
use `curl` so you can follow them without that app.

Examples assume `http://localhost:8080`. Replace it with your server address.
[Sign in](getting-started.md#3-sign-in) once to set the `AUTH` and `JSON` shell
variables used throughout the guides. For every request field and route, open
`/api/v1/docs` on your server.
