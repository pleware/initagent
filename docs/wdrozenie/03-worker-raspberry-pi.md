# 3. Worker na Raspberry Pi — czysty Raspberry Pi OS

Jedna Malina, jeden worker. System **już jest zainstalowany**.
Robisz to w Terminalu. Na końcu wklejasz **jedną** komendę z panelu.

Kilka workerów na jednym Pi → [ścieżka 2](02-worker-pware-os.md)
(Docker prosto na systemie, bez WSL). Windows → [ścieżka 1](01-worker-windows.md).

## Co musisz mieć

Zaznacz w głowie, zanim cokolwiek wkleisz:

1. Raspberry Pi jest włączone i ma **Raspberry Pi OS** (nie Ubuntu, nie Windows).
2. Pi ma internet (otwiera strony albo ping przechodzi).
3. Masz konto: `https://pware.ai` → **Moje konto**, i dostęp do projektu klienta.
4. Umiesz pisać komendy na tym Pi — ekran + klawiatura **albo** SSH z laptopa.

## Jak wejść na Pi

**Wariant A — monitor i klawiatura.**
Na pulpicie otwórz **Terminal** (czarna ikona `>_`).
Na wersji Lite (bez pulpitu) Terminal jest od razu po zalogowaniu.

**Wariant B — z laptopa Windows, bez monitora.**
Otwórz PowerShell i wpisz (zamień `JAN` na użytkownika z Raspberry Pi Imager):

```powershell
ssh JAN@raspberrypi.local
```

Pierwszy raz zapyta o odcisk klucza — wpisz `yes`. Potem hasło z Imager'a.
Jeśli `raspberrypi.local` nie działa, weź adres IP z routera i użyj
`ssh JAN@192.168.x.x`.

Jesteś na Pi, gdy prompt wygląda mniej więcej tak: `JAN@raspberrypi:~ $`.

## Krok 0 — stop, jeśli to nie jest 64 bity

W Terminalu na Pi:

```sh
uname -m
```

Musi wypisać **`aarch64`**.

| Wynik | Co to znaczy |
| --- | --- |
| `aarch64` | OK, jedź dalej |
| `armv7l` albo `armhf` | to 32-bitowy system — **stop**. Wgraj od nowa **Raspberry Pi OS (64-bit)** |
| coś innego | stop i zgłoś wynik |

Produkt nie ma binarki na 32 bity. Stary Pi Zero (nie Zero 2) też odpadnie.

Sprawdź internet:

```sh
ping -c 2 app.initagent.dev
```

Masz zobaczyć odpowiedzi, nie `Network is unreachable`.
Jak nie ma sieci — najpierw Wi-Fi / kabel, dopiero potem reszta.

## Krok 1 — dołóż brakujące programy

```sh
sudo apt-get update
sudo apt-get install -y curl tmux
```

`sudo` zapyta o hasło użytkownika Pi. `curl` pobiera komendę z huba.
`tmux` trzyma terminale, gdy zamkniesz okno.

## Krok 2 — nazwij Pi zanim dołączysz

Panel pokaże workera pod **nazwą komputera** (hostname).
Domyślnie jest `raspberrypi` — zmień to **przed** wklejeniem komendy.

Wymyśl krótką nazwę, np. `sklep-worker-01`. Potem:

```sh
sudo hostnamectl set-hostname sklep-worker-01
```

Albo myszką: menu **Raspberry Pi Configuration → System → Hostname**,
albo `sudo raspi-config` → **System Options → Hostname**.

Potem **restart**:

```sh
sudo reboot
```

Zaloguj się znowu. Sprawdź:

```sh
hostname
```

Musi pokazać nową nazwę. Bez restartu panel dostanie starą.

## Krok 3 — komenda z huba

1. Na laptopie (albo na Pi, jeśli ma przeglądarkę) wejdź na
   `https://pware.ai` i kliknij **Moje konto**. Zaloguj się i otwórz
   projekt klienta.
2. **Add a connector** → wybierz **Linux/macOS** (nie Windows).
3. Skopiuj komendę. Wygląda tak:

```sh
curl -fsSL <ADRES>/install/<TOKEN>.sh | sh
```

`<ADRES>` i `<TOKEN>` pochodzą z panelu — **nie wpisuj ich z pamięci**.
Token żyje **15 minut** i jest jednorazowy. Cała komenda to sekret:
nie wklejaj jej na czat, do maila ani na zrzut ekranu.

## Krok 4 — wklej na Pi (bez sudo)

W Terminalu na Pi wklej skopiowaną komendę i naciśnij Enter.

**Nie dodawaj `sudo`.** Worker ma należeć do zwykłego użytkownika,
tego, którym jesteś zalogowany.

Kolejność na ekranie:

1. `→ downloading the initagent agent for linux/arm64...`
2. `→ enrolling this connector...`
3. `→ installing background service...`
4. `✓ Done.`

Jak stanie na `unsupported architecture` — wróć do kroku 0.
Jak token wygasł — nowa komenda z panelu i wklej jeszcze raz.

## Krok 5 — zostaw workera włączonego po restarcie

Na Pi worker to usługa w tle **Twojego** konta.
Jak zamkniesz SSH albo zrestartujesz Pi bez tego kroku, worker potrafi zgasnąć.

```sh
sudo loginctl enable-linger "$USER"
```

Sprawdź, czy się udało:

```sh
loginctl show-user "$USER" -p Linger
```

Musi być `Linger=yes`.

Potem:

```sh
systemctl --user status initagent-connector
```

Szukaj `active (running)` na zielono. `q` zamyka podgląd.

## Krok 6 — potwierdzenie i smoke test

W panelu pojawia się „**Worker połączony**". To tylko „online".
Nie kończ wdrożenia, dopóki nie wyślesz zadania.

Z panelu: zadanie `initagent version`.
Albo z CLI (token API robisz w Settings → API tokeny):

```sh
initagent fleet login --hub https://app.initagent.dev --token <API_TOKEN>
initagent fleet connectors
initagent fleet run <CONNECTOR> -- initagent version
```

Masz zobaczyć numer wersji, nie błąd.

Na koniec zrestartuj Pi jeszcze raz (`sudo reboot`), zaloguj się
i sprawdź, czy w panelu nadal jest online. Bez tego kroku nie wiesz,
czy worker wstaje sam.

## Co to zrobiło, i o czym wiedzieć

Skrypt pobrał program do `~/.initagent/bin`, dołączył urządzenie
(`agent enroll`) i założył usługę `initagent-connector`.

- **Jedno Pi, jedno konto = jeden worker.** Druga komenda na tym samym
  koncie nadpisze pierwszą — to założenie produktu, nie błąd.
- Worker wstaje przy starcie Pi **tylko** gdy krok 5 pokazał `Linger=yes`.
- **Aktualizacje robi produkt.** Nie podmieniaj pliku `initagent` ręcznie.

## Odinstalowanie

Wpis w hubie zostaje (offline). Usuń go osobno
(`DELETE /api/connectors/{id}`; przycisku w panelu nie ma).

Na Pi:

```sh
systemctl --user disable --now initagent-connector
rm -f ~/.config/systemd/user/initagent-connector.service
rm -rf ~/.initagent
systemctl --user daemon-reload
```

## Gdy coś nie działa

| Objaw | Przyczyna | Co zrobić |
| --- | --- | --- |
| `unsupported architecture` | 32-bitowy system | wgraj Raspberry Pi OS **64-bit**, od kroku 0 |
| `command not found: curl` | brak `curl` | wróć do kroku 1 |
| Panel nie pokazuje maszyny | token wygasł / zużyty | nowa komenda z panelu, powtórz krok 4 |
| Skrypt staje na pobieraniu | brak sieci / DNS | `ping -c 2 app.initagent.dev`; popraw Wi-Fi |
| Online, po restarcie znika | brak linger | powtórz krok 5, zrestartuj, sprawdź panel |
| `active (running)` nie wstaje | zła sesja / usługa | `systemctl --user status initagent-connector` — przepisz błąd |
| Nazwa w panelu to `raspberrypi` | dołączyłeś przed zmianą nazwy | następnym razem krok 2 **przed** 4; teraz tylko `PATCH /api/connectors/{id}` |

## Czego ta instrukcja nie ustala

Otwarte w draftach, nie zgadywane tutaj — tak samo jak w
[części 2](02-worker-pware-os.md):

1. zakres dostępu workera (urządzenie, sieć),
2. sufit zasobów,
3. rozpoznanie hosta, gdy kiedyś staną tu kontenery,
4. czy jedno Pi może obsługiwać projekty różnych organizacji.
