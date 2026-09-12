# 2. PWare OS jako worker na Windows (WSL + Docker)

Ścieżka dla sytuacji, w której na **jednym** Windows ma pracować **kilka**
workerów, każde we własnym środowisku, a obrazy mają zostać w WSL — nie
w Windows.

To jest **konkretny wybór na teraz**, a nie ustalenie produktu. Trzy pytania
z tym związane są jeszcze otwarte i wypisane na końcu; instrukcja mówi, co
robić, ale nie udaje, że jest to rozstrzygnięte.

## Jak to wygląda

```
Windows (jedna maszyna)
└── WSL2: dystrybucja Ubuntu
    ├── systemd (wymagane dla instalacji jako usługa)
    ├── Docker Engine + obrazy i wolumeny (zostają w WSL)
    ├── środowisko 1  → worker A  (własna komenda, własny token, własny dev-)
    ├── środowisko 2  → worker B
    └── środowisko 3  → worker C
                        ↓
              hub app.initagent.dev
```

Window samo nie dołącza niczego. Workerem jest **środowisko w WSL**, bo to
w nim uruchamiasz komendę dołączenia — i tam trafiają zadania.

## Wymagania

- Windows 10 (2004+) albo 11, z włączoną wirtualizacją,
- uprawnienia administratora przy instalacji WSL (dalej już nie),
- konto w hubie z dostępem do projektu klienta.

## Krok 1 — WSL2

```powershell
wsl --install -d Ubuntu
```

Po instalacji zrestartuj maszynę i dokończ pierwsze uruchomienie Ubuntu
(użytkownik + hasło). Sprawdź, że to WSL2:

```powershell
wsl -l -v
```

## Krok 2 — systemd w WSL (wymagane)

Bez systemd instalacja „jako usługa" nie ma się do czego podłączyć; worker
wstałby tylko wtedy, gdy sam uruchomisz `initagent agent run` w otwartym
oknie. Włącz systemd w dystrybucji: w pliku `/etc/wsl.conf` (w Ubuntu):

```ini
[boot]
systemd=true
```

Potem z PowerShella `wsl --shutdown`, wejdź ponownie i sprawdź:

```sh
systemctl --user status
```

Jeśli to działa, `agent install-service` założy jednostkę
`initagent-connector.service`. Dodatkowo włącz trwałość sesji użytkownika,
żeby worker wstał bez otwartego okna WSL:

```sh
loginctl enable-linger "$USER"
```

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

## Krok 4 — jedno środowisko = jeden worker

Dla każdego środowiska, które ma być workerem:

1. W hubie: projekt → **Add device** → **skopiuj komendę dla Linux/macOS**
   (`.sh`). Każde środowisko dostaje **własną** komendę: token jest
   jednorazowy i wygasa po 15 minutach, więc nie da się jednej komendy użyć
   dwa razy.
2. Uruchom ją **wewnątrz tego środowiska** (w dystrybucji WSL albo
   w kontenerze), nie na Windows:

```sh
curl -fsSL <ADRES>/install/<TOKEN>.sh | sh
```

Skrypt robi to samo, co na Windows: pobiera binarkę do `~/.initagent/bin`,
dołącza urządzenie i zakłada usługę. Na Linuksie usługą jest jednostka
systemd — i tu jest **różnica, którą trzeba wybrać świadomie**:

| Wariant | Co to jest | Co za tym idzie |
| --- | --- | --- |
| **A. worker = cała dystrybucja WSL** | instalujesz w Ubuntu, usługa przez systemd | najprościej; środowiska dzielą system plików i sieć |
| **B. worker = kontener** | instalujesz w kontenerze | izolacja; minimalny kontener nie ma systemd, więc nie używaj `install-service`, tylko uruchom `initagent agent run` jako proces główny kontenera (albo obraz z systemd) |

Wariant B jest tym, po który sięgasz, gdy workery mają się nie widzieć.
Który jest właściwy, rozstrzyga odpowiedź na pytanie o izolację (punkt 1
na końcu).

### Jedno konto = jeden worker (inaczej zepsujesz pierwszego)

Konfiguracja workera i nazwa jego usługi siedzą w katalogu domowym konta
(`~/.initagent/connector.json`) oraz w nazwie jednostki. **Druga komenda
wklejona w tym samym koncie nadpisze credential i podmieni usługę pierwszego
workera** — a przy współdzielonym sockecie tmux jeden worker może zobaczyć
i zabić terminale drugiego. Nie jest to usterka instalatora, tylko dzisiejsze
założenie produktu, zapisane w `drafts/10.DRAFT.ENROLL-AND-WORKERS.md`
(`initagent-workspace`) jako „one OS user per project".

Dlatego **osobne konto na każdego workera** — to najtańsza droga i działa bez
żadnych zmian w produkcie:

```sh
sudo adduser worker-a
sudo -iu worker-a '<komenda dołączenia z huba>'
sudo loginctl enable-linger worker-a     # wstaje bez otwartej sesji
```

W wariancie B rolę konta pełni kontener: **jeden kontener = jedno
środowisko**. Hub pokazuje to jako osobne urządzenia `dev-`, dzielące jeden
host — tak produkt modeluje wiele workerów na jednej maszynie. Jak
identyfikowany jest sam host, jest jeszcze otwarte (punkt 3 na końcu).

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

1. **Izolacja workera jako granica bezpieczeństwa.** Czy worker może sięgnąć
   do urządzenia (kamera, drukarka, PLC), do gniazda connector'a, albo do
   sieci firmowej? To rozstrzyga, czy wariant A jest w ogóle dopuszczalny.
   — `pware-os-workspace`, `docs/PWARE-OS-EXAMPLE-MACHINE.md`, sekcja
   *Workers* i jej `Open`.
2. **Kto nadzoruje workery i jaki mają sufit zasobów.** Desk (rozmowa
   z człowiekiem) jest wrażliwy na opóźnienia; build nie. Bez wyraźnego
   sufitu workery potrafią „szarpnąć" interfejsem.
   — jak wyżej.
3. **Identyfikacja fizycznej maszyny.** Trzy dołączenia na jednym Windows:
   hub widzi jedną maszynę czy trzy? Od tego zależy, czy ta instrukcja może
   obiecać „trzy workery na jednej maszynie".
   — `initagent-workspace`, `drafts/10.DRAFT.ENROLL-AND-WORKERS.md`.
4. **Czy jedna maszyna może obsługiwać projekty różnych organizacji.**
   Wdrożeniowiec trzymający kilku klientów na jednej maszynie trafia w to
   od razu. — jak wyżej.
5. **Ile `dev-` na jeden Windows** — czy worker to maszyna, czy środowisko.
   — `initagent-workspace`, `drafts/05.DRAFT.NAMING-ONTOLOGY.md`.

Dopóki 1 i 3 nie są rozstrzygnięte, traktuj tę ścieżkę jako **sprawdzoną
w praktyce, ale nie jako kontrakt**: liczba workerów na maszynie i stopień
ich izolacji mogą się zmienić.
