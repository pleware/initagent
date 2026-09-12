# 1. Worker na Windows — instalacja domyślna

Jedna maszyna, jeden worker. Całość to jedna komenda w PowerShellu.

## Zanim wklejasz: nazwa maszyny

Panel pokaże workera pod nazwą komputera (hostname), zwykle `DESKTOP-4A1B2C`.
Nadaj sensowną nazwę **przed** dołączeniem — zmiana nazwy komputera w Windows
(Ustawienia → System → Informacje) wymaga restartu. Bez restartu: zmień nazwę
urządzenia w hubie (`PATCH /api/devices/{id}`; przycisku w panelu nie ma).

Więcej niż jeden worker na maszynie → [ścieżka 2](02-worker-pware-os.md).

## Krok 1 — komenda z huba

1. Zaloguj się na `https://app.initagent.dev`, otwórz projekt klienta.
2. **Add device** → wybierz **Windows** → skopiuj komendę.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm <ADRES>/install/<TOKEN>.ps1 | iex"
```

`<ADRES>` i `<TOKEN>` pochodzą z panelu — nie wpisuj ich z pamięci.

## Krok 2 — wklej na maszynie

Otwórz PowerShell na maszynie docelowej i wklej. Admin nie jest potrzebny;
uruchom **jako użytkownik, który ma być właścicielem workera** (zadanie wstaje
przy jego zalogowaniu). Zobaczysz `-> downloading …` → `-> enrolling …` →
`-> installing background task…` → `Done.`

## Krok 3 — potwierdzenie i smoke test

W panelu pojawia się „**Worker połączony**". To tylko „online" — sprawdź
wykonaniem: wyślij z panelu zadanie `initagent version`, albo z CLI:

```sh
initagent fleet login --hub https://app.initagent.dev --token <API_TOKEN>
initagent fleet devices
initagent fleet run <DEVICE> -- initagent version
```

`<API_TOKEN>` tworzysz w panelu (Settings → API tokeny). Bez tego kroku nie
melduj wdrożenia jako zakończonego.

## Co to zrobiło, i o czym wiedzieć

Skrypt pobrał binarkę do `%LOCALAPPDATA%\Initagent\bin`, dołączył urządzenie
(`agent enroll`) i założył zadanie w tle (`agent install-service`). Na Windows
„usługa" to **Zadanie harmonogramu `InitagentConnector`, uruchamiane przy
logowaniu** — maszyna bez zalogowanej sesji nie wstanie sama.

- **Jedna maszyna, jedno konto = jeden worker.** Druga komenda w tym samym
  koncie nadpisze pierwszą — to założenie produktu (`drafts/10`), nie błąd.
- **Odinstalowania nie ma.** Ręcznie: usuń zadanie (`schtasks /Delete /TN
  InitagentConnector /F`) i katalog `%LOCALAPPDATA%\Initagent`. Wpis w hubie
  zostaje.
- **Aktualizacje robi produkt** (drenaż → wymiana → start zadania); nie
  podmieniaj binarki ręcznie.

## Gdy coś nie działa

| Objaw | Przyczyna | Co zrobić |
| --- | --- | --- |
| Panel nie pokazuje maszyny | token wygasł / zużyty | nowa komenda, powtórz krok 2 |
| Skrypt staje na pobieraniu | sieć / blokada `irm` | pobierz ręcznie `<ADRES>/install/<TOKEN>.ps1` i `<ADRES>/api/agent-binary?os=windows&arch=amd64`, potem `initagent.exe agent enroll …` + `agent install-service` |
| Online, ale zadanie stoi | brak narzędzia w komendzie | sprawdź log pracy w panelu |
| Nie wstaje po restarcie | zadanie ONLOGON, brak sesji | zaloguj użytkownika |
