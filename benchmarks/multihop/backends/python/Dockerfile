FROM python:3.11-alpine
WORKDIR /app
COPY requirements.txt server.py ./
EXPOSE 9102
ENV PORT=9102
CMD ["python", "server.py"]
