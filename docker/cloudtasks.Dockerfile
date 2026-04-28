FROM python:3.11-slim
RUN pip install --no-cache-dir gcloud-tasks-emulator
EXPOSE 9090
# The Potato London emulator binds to 0.0.0.0 by default when run as a module or if we use the right command.
# Actually, let's use the full python command to ensure it binds to all interfaces if start command is limited.
ENTRYPOINT ["gcloud-tasks-emulator", "start", "-p", "9090"]
