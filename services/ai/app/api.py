import hmac
import os
from uuid import UUID

from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, ConfigDict, Field

from app.extract import DocumentError, Extraction, extract, fixture
from app.retrieve import answer

api = FastAPI(title="NexusAI internal", docs_url=None, redoc_url=None, openapi_url=None)


def authenticate(authorization: str = Header(default="")):
    token = os.environ.get("SERVICE_TOKEN", "")
    if len(token) < 32 or not hmac.compare_digest(
        authorization.encode(), ("Bearer " + token).encode()
    ):
        raise HTTPException(403, "Forbidden")


@api.get("/health")
def health():
    return {"status": "healthy", "mode": "mock"}


class Sample(BaseModel):
    model_config = ConfigDict(extra="forbid")
    sample_id: str


@api.post("/extract", dependencies=[Depends(authenticate)])
def extract_sample(sample: Sample) -> Extraction:
    try:
        return extract(fixture(sample.sample_id))
    except DocumentError as error:
        raise HTTPException(422, str(error)) from None


class Question(BaseModel):
    model_config = ConfigDict(extra="forbid")
    workspace_id: UUID
    question: str = Field(min_length=1, max_length=1000)


@api.post("/retrieve-answer", dependencies=[Depends(authenticate)])
def retrieve_question(req: Question):
    return answer(os.environ["POLICY_DB"], str(req.workspace_id), req.question)
