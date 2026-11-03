# ==========================================
# ЭТАП 1: Сборка (Builder)
# ==========================================
FROM golang:1.27-bookworm AS builder

# Ставим компилятор C (нужен для Ebiten) и заголовочные файлы X11/OpenGL/ALSA
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libgl1-mesa-dev \
    libxcursor-dev \
    libxrandr-dev \
    libxinerama-dev \
    libxi-dev \
    libxext-dev \
    libasound2-dev \
    libxxf86vm-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Сначала копируем только go.mod и go.sum — для кэширования слоев
COPY go.mod go.sum ./
RUN go mod download

# Копируем остальной исходный код
COPY . .

# Собираем бинарник. CGO_ENABLED=1 нужен для работы OpenGL.
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /app/sqwave-mp .

# ==========================================
# ЭТАП 2: Запуск (Runtime)
# ==========================================
FROM debian:bookworm-slim

# Ставим ТОЛЬКО рантайм-библиотеки (без -dev пакетов и компилятора)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libxcursor1 \
    libxrandr2 \
    libxinerama1 \
    libxi6 \
    libxext6 \
    libasound2 \
    libxxf86vm1 \
    libx11-6 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Копируем готовый бинарник из первого этапа
COPY --from=builder /app/sqwave-mp .

# Запускаем игру
ENTRYPOINT ["/app/sqwave-mp"]