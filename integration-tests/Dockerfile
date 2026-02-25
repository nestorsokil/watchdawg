FROM python:3.12-slim

# Docker CLI is needed to read watchdawg container logs via the mounted socket
RUN apt-get update \
    && apt-get install -y --no-install-recommends docker.io \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tests
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .

CMD ["pytest", "tests/", "-v"]
