# 2. PWare OS jako worker na Windows (WSL + Docker)

Kilka workerów na jednej maszynie. **Kształt to decyzja produktu:** tylko
kontenery (Docker w WSL albo na VPS); tryb zwykły to jedna konfiguracja na
maszynę. Czego to nie rozstrzyga — na końcu.

## Jak to wygląda

```
Windows
└── WSL2
    └── Ubuntu (dystrybucja)
        └── Docker Engine (obrazy zostają w WSL)
            ├── kontener 1 → worker A   (własny token, własny dev-)
            ├── kontener 2 → worker B
            └── kontener 3 → worker C
                               ↓
                     hub app.initagent.dev
```

Na VPS jest tak samo, bez WSL: Docker prosto na hoście. Workerem jest zawsze
**kontener** — w nim uruchamiasz komendę dołączenia; sama maszyna nie dołącza
niczego i pozostaje jedną konfiguracją.

## Wymagania

Windows 10 (2004+)/11 z wirtualizacją, admin przy instalacji WSL (dalej już
nie), konto w hubie.

## Krok 0 — nazwij środowisko

Agent bierze nazwę z **hostname'a** (flagi `--name` nie ma). Kontener:
`docker run --hostname dell-worker-01 …`. Dystrybucja WSL: trwale w
`/etc/wsl.conf`:

```ini
[network]
hostname = dell-worker-01
```

potem `wsl --shutdown`. (`hostnamectl` bez systemd nie działa, a sam
`/etc/hostname` przepada po restarcie.) Konwencja: `dell-worker-01`,
`lenovo-worker-02`. Etykieta dystrybucji w `wsl -l -v` (`Ubuntu`) to inna
rzecz — na hub nie wpływa.

## Krok 1 — WSL2 + Ubuntu

```powershell
wsl --install -d Ubuntu
```

Potem **restart Windows** (nie Ubuntu) — `wsl --install` włącza
VirtualMachinePlatform. Po restarcie Ubuntu prosi o użytkownika i hasło
Linuksa (to pierwsze uruchomienie, nie restart); jeśli okno się nie pojawi:
`wsl -d Ubuntu`. Sprawdź tryb: `wsl -l -v` (wersja „2").

## Krok 2 — systemd w WSL (dla Dockera)

Żeby Docker w WSL wstawał sam po restarcie maszyny. W `/etc/wsl.conf` dopisz:

```ini
[boot]
systemd=true
```

potem `wsl --shutdown` z PowerShella (restart **dystrybucji**, nie Windows)
i sprawdź `systemctl status docker`.

## Krok 3 — Docker w WSL

```sh
sudo apt-get update && sudo apt-get install -y docker.io
sudo usermod -aG docker "$USER"   # wyloguj/zaloguj się w WSL
docker run --rm hello-world
```

Obrazy i wolumeny zostają w WSL — nie przenoś ich na Windows, nie stawiaj
Dockera Desktop obok.

## Krok 4 — kontener na workera

Dla każdego środowiska:

1. W hubie: **Add device** → **Linux/macOS** → skopiuj komendę (`.sh`).
   Każdy kontener dostaje własną — token jest jednorazowy, 15 minut.
2. Uruchom ją **wewnątrz kontenera**:

```sh
curl -fsSL <ADRES>/install/<TOKEN>.sh | sh
```

Minimalny kontener nie ma systemd, a skrypt kończy się `agent
install-service` — wtedy użyj `initagent agent run` jako procesu głównego
kontenera (albo obrazu z systemd). Druga komenda w tym samym kontenerze
**nadpisuje credential pierwszego** — kontener na workera, nie dwa w jednym.

Czego tu **nie** robimy: kilku workerów jako kilku kont WSL w jednej
dystrybucji — to działa, ale jest obejściem dla programisty, nie odpowiedzią
dla partnera (`drafts/10`).

## Krok 5 — weryfikacja

Dla każdego środowiska osobno: zadanie z panelu albo
`initagent fleet run <DEVICE> -- initagent version`. Trzy kontenery = trzy
wpisy `dev-` i trzy wyniki do sprawdzenia.

## Utrzymanie

- **Restart Windows:** WSL nie wstaje sam; worker podniesie się, gdy dystrybucja ruszy.
- **Dysk:** `.vhdx` rośnie i nie oddaje miejsca bez `wsl --shutdown` +
  kompaktowania; pilnuj obrazów.
- **Kopie:** odtworzenie kontenera ≠ odtworzenie danych — rób kopie wolumenów.
- **Aktualizacje:** mechanizm produktu; nie podmieniaj binarki ręcznie.

## Odinstalowanie: usunięcie Ubuntu z WSL

`wsl --unregister` **kasuje dystrybucję z całą zawartością** — kontenery,
obrazy, wolumeny, pliki domowe. Kosza nie ma.

1. `docker rm -f <kontener>` — dla każdego środowiska,
2. `wsl --shutdown` (z PowerShella),
3. `wsl --unregister Ubuntu`,
4. `wsl -l -v` — sprawdź; jeśli Ubuntu było domyślną, `wsl --set-default <inna>`,
5. **wpis w hubie zostaje** jako offline — usuń go `DELETE /api/devices/{id}`
   (przycisku w panelu nie ma).

Na VPS bez WSL: `docker rm -f` + `docker rmi`.

## Czego ta instrukcja nie ustala

Otwarte w draftach, nie zgadywane tutaj:

1. **zakres dostępu** — co worker może sięgnąć (urządzenie, gniazdo connectora,
   sieć firmowa) — `pware-os-workspace`, `docs/PWARE-OS-EXAMPLE-MACHINE.md` § *Workers*;
2. **sufit zasobów** — desk jest wrażliwy na opóźnienia, build nie — j.w.;
3. **rozpoznanie hosta** — kilka `dev-` dzielących jedną maszynę —
   `initagent-workspace`, `drafts/10`;
4. **tenancy** — czy jedna maszyna może obsługiwać projekty różnych organizacji — j.w.;
5. **nazewnictwo** — czy `dev-` to maszyna, czy środowisko — `drafts/05`.

Dopóki 1–3 nie są rozstrzygnięte: ścieżka jest sprawdzona w praktyce, nie
kontraktem.
