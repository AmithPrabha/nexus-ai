import hashlib
import math
import re
from collections import Counter

import psycopg

STOP = set(
    "a an and are as at be before by can do does for from how i in is it many much my of on or our that the their to what when where which who with within should must need needs we they have has this according company policy policies fictional does document documents me tell".split()
)
ALIASES = {
    "vendor": "supplier",
    "vendors": "supplier",
    "purchasing": "purchase",
    "purchases": "purchase",
    "approves": "approve",
    "approved": "approve",
    "approval": "approve",
    "approver": "approve",
    "holiday": "leave",
    "vacation": "leave",
    "expenses": "expense",
    "quotations": "quote",
    "quotation": "quote",
    "quotes": "quote",
    "receipts": "receipt",
    "invoices": "invoice",
    "trips": "travel",
    "orders": "order",
    "employees": "employee",
    "rejected": "rejection",
    "booking": "book",
    "books": "book",
    "days": "day",
}
VERSION = "mock-hash-96-v1"
ABSTAIN = {
    "answer": "The assigned policies provide insufficient evidence.",
    "abstained": True,
    "citations": [],
    "reason": "insufficient_evidence",
}


def tokens(text: str) -> list[str]:
    return [
        ALIASES.get(w, w)
        for w in re.findall(r"[a-z0-9]+", text.lower())
        if w not in STOP
    ]


def embedding(text: str) -> str:
    vector = [0] * 96
    for word, count in Counter(tokens(text)).items():
        digest = hashlib.sha256(word.encode()).digest()
        vector[digest[0] % 96] += count if digest[1] % 2 else -count
    length = math.sqrt(sum(x * x for x in vector)) or 1
    return "[" + ",".join(f"{x / length:.7f}" for x in vector) + "]"


def answer(dsn: str, workspace: str, question: str) -> dict:
    words = set(tokens(question))
    if not words or re.search(
        r"\b(ignore|override instructions|system prompt|disregard|pretend|reveal|execute)\b",
        question,
        re.I,
    ):
        return dict(ABSTAIN)
    with psycopg.connect(
        dsn,
        connect_timeout=3,
        options="-c statement_timeout=3000 -c default_transaction_read_only=on",
    ) as db:
        db.execute("SELECT set_config('nexus.tenant',%s,true)", (workspace,))
        scope = """ FROM policy_chunks c JOIN workspace_policies w ON w.policy_id=c.policy_id
            WHERE w.workspace_id=%s AND c.embedding_version=%s """
        vector = db.execute(
            "SELECT c.id" + scope + " ORDER BY embedding <=> %s::vector,c.id LIMIT 10",
            (workspace, VERSION, embedding(question)),
        ).fetchall()
        query = " | ".join(sorted(words))
        keyword = db.execute(
            "SELECT c.id"
            + scope
            + """ AND search_vector @@ to_tsquery('english',%s)
             ORDER BY ts_rank_cd(search_vector,to_tsquery('english',%s)) DESC,c.id LIMIT 10""",
            (workspace, VERSION, query, query),
        ).fetchall()
        scores = Counter()
        for ranking in (vector, keyword):
            for rank, (chunk,) in enumerate(ranking, 1):
                scores[chunk] += 1 / (60 + rank)
        top = [chunk for chunk, _ in scores.most_common(5)]
        context = db.execute(
            "SELECT c.id,c.topic,c.body" + scope + " AND c.id=ANY(%s::uuid[])",
            (workspace, VERSION, top),
        ).fetchall()
    # A mock answer quotes one clause. Require meaningful lexical coverage and a clear lead.
    ranked = []
    for chunk, topic, body in context:
        overlap = words.intersection(tokens(topic + " " + body))
        ranked.append((len(overlap), scores[chunk], str(chunk), body))
    ranked.sort(reverse=True)
    if not ranked:
        return dict(ABSTAIN)
    count, _, chunk, body = ranked[0]
    if (
        count < 2
        or count / len(words) < 0.48
        or (len(ranked) > 1 and count == ranked[1][0])
    ):
        return dict(ABSTAIN)
    return {
        "answer": body,
        "abstained": False,
        "citations": [{"chunk_id": chunk}],
        "reason": "quoted_clause",
    }
