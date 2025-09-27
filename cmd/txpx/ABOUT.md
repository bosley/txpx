# TXPX

The txpx app is something i'm playing around with for insi orch and mgmt
Right now, when the app come sup it launches insi, just like insid but with
default configurations baked-in for ease of use.

Once the app and insi are started, we monitor insi activity via the insi client
and emit onto the local event system (non-insi) system channel that the cluster/node
is reachable or not

The insi client in-use leverages the cluster config directly, meaning its a root-access
level controller.