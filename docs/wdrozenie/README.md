# Wdrożenie workera — instrukcja dla partnera wdrożeniowego

Ta instrukcja jest dla partnera, który na maszynie klienta ma **podłączyć
workera** do huba initAgent. Nie opisuje stawiania huba: hub stoi na
`app.initagent.dev` i nie jest częścią tej instalacji.

Wybierz jedną z dwóch ścieżek:

| Ścieżka | Kiedy ją wybierasz | Dokument |
| --- | --- | --- |
| **1. Instalacja domyślna** | jedna maszyna (Windows, Linux albo macOS) ma być jednym workerem | [`01-worker-windows.md`](01-worker-windows.md) |
| **2. PWare OS jako worker** | na jednej maszynie ma działać kilka workerów — **wyłącznie w kontenerach** (Docker w WSL albo na VPS), obrazy zostają w WSL | [`02-worker-pware-os.md`](02-worker-pware-os.md) |

Tryb zwykły to **jedna konfiguracja na maszynę**. Kilka workerów na jednej
maszynie uruchamiamy tylko jako kontenery — tak brzmi decyzja produktu, nie
nasze uproszczenie.

Obie ścieżki kończą się tym samym: maszyna (albo środowisko) pojawia się
w hubie jako online i wykonuje zadanie. Różnią się tym, ile środowisk
uruchamiasz i gdzie.

## Słownik

- **worker** — maszyna albo środowisko z uruchomionym connector'em. W hubie to
  jedno urządzenie z identyfikatorem `dev-`.
- **hub** — panel i API na `app.initagent.dev`. Tu wybierasz projekt klienta
  i stąd bierzesz komendę wdrożenia.
- **token dołączenia** — **jednorazowy, wygasa po 15 minutach**. Jest wbudowany
  w komendę, więc **sama komenda jest sekretem**: nie wklejaj jej do czatu
  zespołowego ani do zgłoszenia, a jeśli wyciekła — wygeneruj nową.
- W komendach zobaczysz słowo `agent` (`initagent agent enroll`). To ten sam
  komponent, który produkt nazywa **workerem**; instrukcja trzyma się słowa
  „worker", bo tak mówi interfejs („Worker połączony") i dokumentacja.

## Czego potrzebujesz przed startem

- konto na `app.initagent.dev` z dostępem do projektu klienta,
- dostęp do maszyny docelowej (konsola, RDP albo SSH),
- 15 minut na wklejenie komendy, zanim token wygaśnie — komenda powstaje
  w momencie pokazania jej w panelu.

## Czego ta instrukcja świadomie nie ustala

Trzy pytania produktu są jeszcze otwarte i dotykają tej instrukcji wprost.
Nie zgadujemy ich tutaj; są opisane w draftach i linkowane na końcu każdej
części:

1. jak identyfikowana jest **fizyczna maszyna**, gdy na jednym hoście stoi
   kilka kontenerów: hub musi wiedzieć, że to jedna maszyna, a nie trzy,
2. **co worker może sięgnąć** — urządzenie, gniazdo connectora, sieć firmowa —
   oraz jaki ma sufit zasobów. Mechanizm dla kilku workerów jest wybrany
   (kontener, na rzecz izolacji); otwarty jest zakres, nie wybór,
3. czy jedna maszyna może obsługiwać projekty **różnych organizacji**.

Język: polski. Wersja angielska dojdzie później; [`README.md`](../../README.md)
repo opisuje po angielsku instalację samego huba.
