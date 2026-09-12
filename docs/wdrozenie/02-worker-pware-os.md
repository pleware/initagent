# 2. PWare OS jako worker na Windows (WSL + Docker)

Ścieżka dla sytuacji, w której na **jednej** maszynie ma pracować **kilka**
workerów, każde we własnym środowisku, a obrazy mają zostać w WSL — nie
w Windows.

**Kształt tej ścieżki jest decyzją produktu:** kilka workerów na jednej
maszynie uruchamiamy **tylko w kontenerach** (Docker w WSL albo na VPS), a tryb
zwykły to jedna konfiguracja na maszynę. Czego ta decyzja nie rozstrzyga —
zakresu dostępu workera, sufitu zasobów i identyfikacji hosta — jest wypisane
na końcu.

## Jak to wygląda

```
Maszyna (Windows z WSL2, albo VPS)
└── Docker Engine (obrazy i wolumeny zostają tutaj)
    ├── kontener 1 → worker A  (własna komenda, własny token, własny dev-)
    ├── kontener 2 → worker B
    └── kontener 3 → worker C
                        ↓
              hub app.initagent.dev
```

Na Windows Docker siedzi **w WSL**, bo tam ma zostać to, co waży — obrazy
i warstwy. Na VPS jest po prostu na hoście. W obu wypadkach workerem jest
**kontener**, bo w nim uruchamiasz komendę dołączenia i tam trafiają zadania.
Sama maszyna nie dołącza niczego i pozostaje jedną konfiguracją.

## Wymagania

- Windows 10 (2004+) albo 11, z włączoną wirtualizacją,
- uprawnienia administratora przy instalacji WSL (dalej już nie),
- konto w hubie z dostępem do projektu klienta.

## Krok 1 — WSL2

```powershell
wsl --install -d Ubuntu
```

Potem **restart Windows, nie Ubuntu**. `wsl --install` włącza funkcje systemu
(VirtualMachinePlatform), a Windows tego nie dokończy bez restartu. To jest
pierwszy z dwóch „restartów" w tej instrukcji i dotyczy **hosta**.

Po restarcie Ubuntu zwykle samo otwiera konsolę i prosi o **użytkownika
i hasło Linuksa** — to pierwsze uruchomienie dystrybucji, nie kolejny restart.
Jeśli okno się nie pojawi: `wsl -d Ubuntu` z PowerShella albo „Ubuntu" z menu
Start.

Sprawdź, że dystrybucja pracuje w trybie WSL2:

```powershell
wsl -l -v
```

W całej instrukcji „restart" znaczy **restart Windows albo `wsl --shutdown`
z PowerShella**. W środku Ubuntu nie ma czego restartować — drugi taki moment
jest w kroku 2, po zmianie `/etc/wsl.conf`.

## Krok 2 — systemd w WSL (dla Dockera)

Tu nie chodzi o workera — ten siedzi w kontenerze — tylko o to, żeby Docker
w WSL wstawał sam po restarcie maszyny. Bez systemd trzeba go podnosić ręcznie
w otwartym oknie. Włącz systemd w dystrybucji, w pliku `/etc/wsl.conf`:

```ini
[boot]
systemd=true
```

Potem z PowerShella `wsl --shutdown` (to restart dystrybucji, nie Windows),
wejdź ponownie i sprawdź:

```sh
systemctl status docker
```

Gdyby workerem miała być jednak sama dystrybucja, a nie kontener, to jest
właśnie to miejsce, w którym `agent install-service` założy jednostkę
`initagent-connector.service`, a `loginctl enable-linger "$USER"` utrzyma ją
bez otwartej sesji. To jednak ścieżka dla **jednego** workera — dla kilku
kontenery, jak niżej.

## Krok 3 — Docker w WSL

Docker instalujesz **w środku dystrybucji**, nie w Windows:

```sh
sudo apt-get update && sudo apt-get install -y docker.io
sudo usermod -aG docker "$USER"   # potem wyloguj/zaloguj się w WSL
docker run --rm hello-world
```

Świadomie: **obrazy, wolumeny i warstwy zostają w WSL**. Nie przenoś ich na
dysk Windows i nie stawiaj Dockera Desktop obok — trzymanie obrazów po stronie
WSL jest tym, po co ta ścieżka istnieje.

## Krok 4 — jedno środowisko = jeden kontener

**Kilku workerów na jednej maszynie robimy wyłącznie w kontenerach.** Tak brzmi
decyzja produktu (`initagent-workspace/drafts/10.DRAFT.ENROLL-AND-WORKERS.md`):
Docker — na VPS albo w WSL — jest jedynym kształtem, który publikujemy dla
kilku workerów na jednej maszynie. Tryb zwykły (instalacja natywna, część 1)
to **jedna konfiguracja na maszynę**, i druga komenda wklejona w tym samym
miejscu nadpisze pierwszą — to zachowanie zamierzone, nie usterka do obejścia.

Dla każdego kontenera:

1. W hubie: projekt → **Add device** → **skopiuj komendę dla Linux/macOS**
   (`.sh`). Każdy kontener dostaje **własną** komendę: token jest
   jednorazowy i wygasa po 15 minutach.
2. Uruchom ją **wewnątrz kontenera** — nie na maszynie i nie w dystrybucji
   WSL obok:

```sh
curl -fsSL <ADRES>/install/<TOKEN>.sh | sh
```

**Uwaga o usłudze w kontenerze.** Na Linuksie skrypt kończy się
`agent install-service`, a to znaczy „jednostka systemd". Minimalny kontener
systemd nie ma — wtedy nie używaj `install-service`, tylko uruchom
`initagent agent run` jako proces główny kontenera (albo weź obraz z systemd).

### Jeden kontener = jedno środowisko

Konfiguracja workera i nazwa jego usługi siedzą w katalogu domowym konta
(`~/.initagent/connector.json`) oraz w nazwie jednostki. **Druga komenda
wklejona w tym samym kontenerze nadpisze credential pierwszego workera** —
a przy wspólnym sockecie tmux jeden worker może zobaczyć i zabić terminale
drugiego. Dlatego **kontener na workera**, nie dwa workery w jednym.

Hub pokazuje to jako osobne urządzenia `dev-`, dzielące jeden host. Jak
identyfikowany jest sam host, jest jeszcze otwarte (punkt 3 na końcu).

**Czego w tej ścieżce nie robimy:** kilku workerów jako kilku kont użytkownika
w jednej dystrybucji WSL. To działa — osobny katalog domowy daje osobny
config, osobną usługę i osobny socket — ale jest obejściem dla programisty
przy klawiaturze, a nie odpowiedzią, którą dajemy partnerowi. Tak jest to
zapisane w `drafts/10`: jeden OS user na projekt zostaje dozwolonym
workaroundem, natomiast udokumentowanym kształtem wielu workerów jest
kontener.

## Krok 5 — weryfikacja

Dla **każdego** środowiska osobno, jak w części 1, krok 4: wyślij zadanie
z panelu (albo `initagent fleet run <DEVICE> -- initagent version`). Trzy
środowiska na maszynie to trzy wpisy `dev-` w projekcie i trzy wyniki do
sprawdzenia.

## Utrzymanie

- **Restart Windows:** WSL nie wstaje sam. Worker podniesie się, gdy
  dystrybucja zostanie uruchomiona (dlatego linger z kroku 2 i zadanie
  uruchamiane przy logowaniu).
- **Dysk:** plik `.vhdx` dystrybucji rośnie i nie oddaje miejsca bez
  `wsl --shutdown` + kompaktowania. Obrazy Docker to zwykle główny
  konsument — pilnuj tego przed wdrożeniem u klienta.
- **Kopie/obrazy:** trzymaj je w WSL, ale rób kopię tego, co nieodtwarzalne
  (wolumeny z danymi), bo odtworzenie kontenera to nie odtworzenie danych.
- **Aktualizacje:** przez mechanizm produktu (drenaż → wymiana → start
  jednostki), nie przez ręczną podmianę binarki.
- **Odinstalowanie:** `systemctl --user disable --now initagent-connector`,
  usuń `~/.initagent`. Wpis urządzenia w hubie zostaje — produkt nie ma
  jeszcze ścieżki odłączenia (otwarty punkt w `drafts/10`).

## Czego ta instrukcja nie ustala (i to jest ważne)

Kształt jest rozstrzygnięty — kontenery. Otwarte jest to, co worker może, ile
mu wolno i jak hub widzi samą maszynę:

1. **Co worker może sięgnąć.** Urządzenie (kamera, drukarka, PLC), gniazdo
   connectora, sieć firmowa. Mechanizm dla wielu workerów jest wybrany na
   rzecz izolacji, więc pytanie nie brzmi już „kontener czy nie", tylko
   „jaki zakres". — `pware-os-workspace`,
   `docs/PWARE-OS-EXAMPLE-MACHINE.md`, sekcja *Workers* i jej `Open`.
2. **Kto nadzoruje workery i jaki mają sufit zasobów.** Desk (rozmowa
   z człowiekiem) jest wrażliwy na opóźnienia; build nie. Bez wyraźnego sufitu
   workery potrafią „szarpnąć" interfejsem. — jak wyżej.
3. **Identyfikacja fizycznej maszyny.** Kilka kontenerów na jednej maszynie to
   kilka wpisów `dev-` dzielących jeden host: hub musi wiedzieć, że to jedna
   maszyna, a nie trzy. — `initagent-workspace`,
   `drafts/10.DRAFT.ENROLL-AND-WORKERS.md`.
4. **Czy jedna maszyna może obsługiwać projekty różnych organizacji.**
   Wdrożeniowiec trzymający kilku klientów na jednej maszynie trafia w to
   od razu. — jak wyżej.
5. **Nazewnictwo: worker to maszyna czy środowisko.** Skoro workerem jest
   kontener, a host jest osobno, to rozstrzygnięcie przesądza, co nazywa `dev-`.
   — `initagent-workspace`, `drafts/05.DRAFT.NAMING-ONTOLOGY.md`.

Dopóki 1–3 nie są rozstrzygnięte, traktuj tę ścieżkę jako **sprawdzoną
w praktyce, ale nie jako kontrakt**: zakres dostępu workera, jego sufit
zasobów i sposób, w jaki hub rozpoznaje host, mogą się zmienić.
