# Wdrożenie workera — instrukcja dla partnera wdrożeniowego

Jak podłączyć **workera** do huba initAgent na maszynie klienta. Hub stoi na
`app.initagent.dev` i nie jest częścią tej instalacji.

| Ścieżka | Kiedy | Dokument |
| --- | --- | --- |
| **1. Domyślna** | jedna maszyna = jeden worker | [`01-worker-windows.md`](01-worker-windows.md) |
| **2. PWare OS** | kilka workerów na jednej maszynie — **tylko kontenery** (Docker w WSL albo na VPS) | [`02-worker-pware-os.md`](02-worker-pware-os.md) |

## Komendy w skrócie

`<ADRES>` i `<TOKEN>` to adres i jednorazowy token z panelu — nie wpisuj ich
z pamięci; token wygasa po 15 minutach.

| Komenda | Po co | Gdzie |
| --- | --- | --- |
| `irm <ADRES>/install/<TOKEN>.ps1 \| iex` | dołącza workera (Windows) | cz. 1 |
| `curl -fsSL <ADRES>/install/<TOKEN>.sh \| sh` | dołącza workera (Linux/kontener) | cz. 2 |
| `wsl --install -d Ubuntu` | zakłada WSL2 + Ubuntu | cz. 2 |
| `wsl --shutdown` | restart dystrybucji po zmianie `/etc/wsl.conf` | cz. 2 |
| `wsl -l -v` | lista dystrybucji / tryb WSL | cz. 2 |
| `systemctl status docker` | czy Docker w WSL wstaje sam | cz. 2 |
| `docker run --hostname dell-worker-01 …` | kontener o jawnej nazwie = worker | cz. 2 |
| `initagent fleet connectors` | lista urządzeń (weryfikacja) | obie |
| `initagent fleet run <CONNECTOR> -- initagent version` | smoke test | obie |
| `wsl --unregister Ubuntu` | **kasuje** dystrybucję i całą jej zawartość | cz. 2 |

## Słownik

- **worker** — maszyna albo środowisko z connector'em; w hubie to jedno
  urządzenie `dev-`.
- **hub** — panel i API na `app.initagent.dev`; stąd bierzesz komendę.
- **token dołączenia** — jednorazowy, **wygasa po 15 minutach**, wbudowany
  w komendę — więc **komenda jest sekretem**.
- W komendach zobaczysz `agent` (`initagent agent enroll`) — to ten sam
  komponent, co worker; interfejs mówi „Worker połączony".

## Zanim zaczniesz

- konto na `app.initagent.dev` z dostępem do projektu klienta,
- dostęp do maszyny (konsola, RDP, SSH),
- nazwa workera: agent bierze ją z **hostname'a** środowiska, więc ustal ją
  przed dołączeniem (`dell-worker-01`) — mechanika w każdej części. Hub umie
  ją zmienić po fakcie (`PATCH /api/connectors/{id}`), ale panel nie ma na to
  przycisku.

## Czego ta instrukcja świadomie nie ustala

1. jak hub rozpoznaje **fizyczną maszynę**, gdy stoi na niej kilka kontenerów,
2. **co worker może sięgnąć** (urządzenie, gniazdo connectora, sieć firmowa)
   i jaki ma sufit zasobów,
3. czy jedna maszyna może obsługiwać projekty **różnych organizacji**.

Otwarte w draftach; linkowane na końcu każdej części.

Język: polski. [`README.md`](../../README.md) repo opisuje po angielsku
instalację samego huba.
