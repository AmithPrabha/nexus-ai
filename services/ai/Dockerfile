FROM ghcr.io/astral-sh/uv:0.8.15 AS uv
FROM python:3.12-slim AS runtime
COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /srv
COPY services/ai/pyproject.toml services/ai/uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project
ENV PATH="/srv/.venv/bin:$PATH" PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY services/ai/app ./app
USER 65532:65532
CMD ["uvicorn","app.api:api","--host","0.0.0.0","--port","8000","--no-access-log"]

FROM runtime AS checks
USER root
RUN uv sync --frozen --no-install-project
COPY services/ai/ruff.toml ./
COPY services/ai/tests ./tests
COPY fixtures /fixtures
COPY packages/contracts/ai.openapi.json /packages/contracts/ai.openapi.json
RUN ruff check app tests && pytest -q

FROM runtime AS service
