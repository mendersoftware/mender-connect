FROM golang:1.21 as builder

WORKDIR /nt-connect

COPY . .

FROM builder as builder-deps

RUN apt update && apt install -qy $(cat deb-requirements.txt)

FROM builder-deps as builder-build

RUN go build -o nt-connect

FROM python:3.11-slim

RUN apt update && apt install -qy libglib2.0-dev

COPY --from=builder-build /nt-connect/nt-connect /usr/bin/nt-connect
COPY requirements.txt /requirements.txt
COPY examples/mender-connect.conf /etc/mender/mender-connect.conf


RUN pip install -r requirements.txt
COPY entrypoint.py /entrypoint.py
ENTRYPOINT ["python3", "./entrypoint.py", "daemon"]
