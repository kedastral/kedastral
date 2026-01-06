FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

EXPOSE 8082

CMD ["gunicorn", "-w", "4", "-b", "0.0.0.0:8082", "--timeout", "60", "app:app"]
