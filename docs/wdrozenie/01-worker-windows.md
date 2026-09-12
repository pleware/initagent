# 1. Worker na Windows — instalacja domyślna

Ścieżka dla jednej maszyny, która ma być **jednym** workerem. Cała instalacja
to jedna komenda wklejona w PowerShell.

## Krok 1 — weź komendę z huba

1. Zaloguj się na `https://app.initagent.dev`.
2. Otwórz projekt klienta.
3. Kliknij **Add device** (przy pierwszym projekcie ten sam ekran pojawia się
   po rejestracji, jako krok boardingu).
4. Wybierz **Windows** i skopiuj pokazaną komendę.

Komenda ma postać:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm <ADRES>/install/<TOKEN>.ps1 | iex"
```

`<ADRES>` to adres twojego huba, `<TOKEN>` to jednorazowy token. **Nie wpisuj
tego z pamięci i nie skracaj** — adres i token pochodzą z panelu.

## Krok 2 — wklej ją na maszynie docelowej

Otwórz PowerShell na maszynie, która ma zostać workerem, i wklej komendę.

- Podniesienie uprawnień nie jest potrzebne: skrypt pisze do
  `%LOCALAPPDATA%` i zakłada zadanie dla bieżącego użytkownika.
- Uruchom to **jako użytkownik, który ma być właścicielem workera** (patrz
  „Konsekwencje" niżej). Nie instaluj workera pod kontem, na którym nikt się
  nie loguje.

W oknie zobaczysz kolejno:

```
-> downloading the initagent agent for windows/amd64...
-> enrolling this device...
-> installing background task...

Done. This device is now connected to initagent.
It should appear on your dashboard within a few seconds.
```

## Krok 3 — poczekaj na potwierdzenie w hubie

W panelu, na tym samym ekranie, pojawia się „**Worker połączony**". Jeśli
token wygasł (15 minut), ekran tego nie zobaczy — wygeneruj nową komendę
i powtórz krok 2. Token jest jednorazowy: drugie użycie tej samej komendy
się nie powiedzie.

## Krok 4 — sprawdź, że worker wykonuje pracę

Samo „online" znaczy tylko, że połączenie wstało. Potwierdź wykonaniem:

- **z panelu:** wyślij jedno zadanie z ekranu zadań (np. `initagent version`)
  i zobacz wynik,
- **albo z CLI** na dowolnej maszynie z binarką:

```sh
initagent fleet login --hub https://app.initagent.dev --token <API_TOKEN>
initagent fleet devices
initagent fleet run <DEVICE> -- initagent version
```

`<API_TOKEN>` tworzysz w panelu (Settings → API tokeny). Bez tego kroku nie
melduj wdrożenia jako zakończonego.

## Co ta komenda właściwie zrobiła

Warto to wiedzieć, bo klient zapyta:

1. wykryła architekturę (`amd64`/`arm64`),
2. pobrała binarkę z `<ADRES>/api/agent-binary?os=windows&arch=<arch>` do
   `%LOCALAPPDATA%\Initagent\bin\initagent.exe`,
3. zapisała urządzenie w hubie: `initagent agent enroll --hub <ADRES> --token <TOKEN>`,
4. zainstalowała zadanie w tle: `initagent agent install-service`.

Na Windows „usługa" to **Zadanie harmonogramu o nazwie `InitagentConnector`,
uruchamiane przy zalogowaniu użytkownika** (nie usługa systemowa). Na Linuksie
to jednostka systemd (`initagent-connector.service`), a na macOS agent
launchd. Data workera siedzi w `%LOCALAPPDATA%\Initagent`.

## Konsekwencje, o których trzeba wiedzieć

- **Zadanie wstaje przy zalogowaniu.** Maszyna, na której nikt się nie
  zaloguje (np. serwer bez sesji), nie podniesie workera sama. Jeśli to
  wymaganie, powiedz o tym przed wdrożeniem — to nie jest dziś przełącznik
  w instalatorze.
- **Jedna maszyna, jedno konto = jeden worker.** Druga komenda wklejona
  w tym samym koncie **nadpisze** konfigurację i zadanie pierwszego
  (`%LOCALAPPDATA%\Initagent` i zadanie `InitagentConnector` są wspólne dla
  konta). To nie usterka instalatora, tylko dzisiejsze założenie produktu —
  opisane w `drafts/10.DRAFT.ENROLL-AND-WORKERS.md` (`initagent-workspace`)
  jako „one OS user per project". Kilku workerów na jednej maszynie to
  [ścieżka 2](02-worker-pware-os.md).
- **Odinstalowania nie ma.** Produkt nie ma jeszcze ścieżki „usuń workera"
  (otwarty punkt w `drafts/10`). Ręcznie: zatrzymaj i usuń zadanie
  (`schtasks /End /TN InitagentConnector`, potem
  `schtasks /Delete /TN InitagentConnector /F`) i usuń katalog
  `%LOCALAPPDATA%\Initagent`. Wpis maszyny w hubie zostaje — nie kasuj go
  ręcznie w bazie.
- **Aktualizacje robi produkt.** Nie podmieniaj binarki w trakcie pracy
  workera; aktualizacja to drenaż, wymiana i ponowny start przez to samo
  zadanie.

## Gdy coś nie działa

| Objaw | Najczęstsza przyczyna | Co zrobić |
| --- | --- | --- |
| Panel nie pokazuje maszyny | token wygasł albo zużyty | wygeneruj nową komendę i powtórz krok 2 |
| Skrypt zatrzymał się na pobieraniu | brak wyjścia w sieci albo polityka blokuje `irm` | pobierz ręcznie `<ADRES>/install/<TOKEN>.ps1` i `<ADRES>/api/agent-binary?os=windows&arch=amd64`, potem uruchom `initagent.exe agent enroll --hub <ADRES> --token <TOKEN>` i `... agent install-service` |
| Maszyna online, zadanie nie kończy się | brak narzędzia, którego wymaga komenda | sprawdź log pracy w panelu; to nie problem workera |
| Worker nie wstaje po restarcie | zadanie jest ONLOGON, a nikt się nie zalogował | zaloguj użytkownika albo zaplanuj sesję |

## Gdy potrzebujesz kilku workerów na jednej maszynie

To już ścieżka druga: [`02-worker-pware-os.md`](02-worker-pware-os.md).
Domyślna instalacja zakłada **jedną maszynę = jednego workera** i przy dwóch
dołączeniach na jednym Windows hub może zobaczyć jedną maszynę zamiast dwóch
— to otwarta decyzja produktu, opisana w `drafts/10` (`initagent-workspace`).
