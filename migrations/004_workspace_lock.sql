-- PostgreSQL requires an UPDATE privilege to acquire the workspace row lock.
GRANT UPDATE(expires_at) ON workspaces TO nexus_gateway;
