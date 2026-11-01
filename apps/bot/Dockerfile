FROM python:3.12-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    BOT_HEARTBEAT_FILE=/tmp/kuberan-bot-heartbeat

WORKDIR /app

COPY requirements.txt .
RUN pip install -r requirements.txt

COPY main.py config.py api_client.py ./
COPY handlers/ ./handlers/
COPY utils/ ./utils/

# Unprivileged runtime user; the app only needs to read /app and write the
# heartbeat file under /tmp (a tmpfs in docker-compose.prod.yml).
RUN groupadd --system --gid 10001 kuberan \
    && useradd --system --uid 10001 --gid kuberan --no-create-home --shell /usr/sbin/nologin kuberan
USER 10001:10001

# Healthy while the job-queue heartbeat (every 30s) is fresh: catches a wedged
# event loop or dead polling, not just a missing process.
HEALTHCHECK --interval=60s --timeout=5s --start-period=60s --retries=3 \
    CMD ["python", "-c", "import os,sys,time; p=os.environ['BOT_HEARTBEAT_FILE']; sys.exit(0 if os.path.exists(p) and time.time()-os.path.getmtime(p) < 120 else 1)"]

CMD ["python", "main.py"]
