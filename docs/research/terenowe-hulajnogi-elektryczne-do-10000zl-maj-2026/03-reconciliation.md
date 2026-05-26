# 03-reconciliation.md

## Zakres kontroli
Porównano:
- `01-exploration.md`
- `02-rynek-katalog-polska.md`
- `02-technika-teren-zasieg.md`
- `02-serwis-niezawodnosc-polska.md`
- `00-candidate-ledger.md`
- planowany raport końcowy

## 1. Normalizacja zakresu
Stary research w repo używał węższego rozumienia „terenowe” i realnej ceny sklepowej. W nowym researchu zakres został ujednolicony do:
- terenowe + all-terrain,
- cena katalogowa / regularna,
- modele nielegalne drogowo dopuszczone,
- serwis w Polsce ważny, ale niewykluczający,
- priorytety: osiągi w terenie, zasięg, niezawodność.

Luka zakresowa została zamknięta w nowych plikach `01-*` i `00-*`.

## 2. Kontrola kompletności encji
Sprawdzono, czy każda istotna encja z eksploracji i plików kątowych ma wiersz w ledgerze.

### Encje głównych modeli — obecne w ledgerze
- Hiley Tiger 10 V5 / Performance / EVO GT / GTR
- Techlife Q7 / Q7 RS / Q7 RSX
- Techlife Q5 / Q5 RS / Q5 RSX
- Teverun Fighter Mini Pro
- Vsett 10+
- Kaabo Mantis 10 / 10 Pro
- KuKirin G2 Master / G3 Pro / G4 / G4 Max
- Joyor S10-S
- Motus Pro 10 GT / Pro 10 Sport GT
- Nami Klima
- Dualtron Popular
- Ausom DT2 Pro
- Obarter D5

### Encje kanałowe / serwisowe — obecne w ledgerze jako supporting / benchmark-only
- Hiley Polska
- Techlife / Mobiway
- Teverun Polska
- Motus
- MAJway
- KuKirin Polska oficjalne kanały
- 7Way / Electricall / Bikeforce

Wniosek: na poziomie istotnych encji **nie ma brakujących bytów wpływających na syntezę**.

## 3. Normalizacja aliasów i wariantów
### Zamknięte normalizacje
- **Techlife Q7 / Q7 RS / Q7 RSX** ↔ **Teverun Fighter Mini 20 / 25 / 30Ah** na podstawie oficjalnej strony Teverun Polska.
- **Techlife Q5 / Q5 RS / Q5 RSX** ↔ **Teverun Blade Mini 20 / 25 / 30Ah** na podstawie oficjalnej strony Teverun Polska.
- **Kaabo Mantis 10 Pro** i **Kaabo Mantis Pro** potraktowano jako praktyczny klaster porównawczy, ale z notą o niejednoznaczności polskiej dostępności konkretnego wariantu.
- **Nami Klima / Klima Max** potraktowano jako wspólną rodzinę benchmarkową z jawnym zaznaczeniem konfliktu budżetowego.

### Niezamknięte lub ostrożne normalizacje
- **KuKirin G2 Pro ABE / VMP / G2 VMP** pozostają przede wszystkim bytem porządkującym nazewnictwo, nie modelem końcowej shortlisty.
- **KuKirin dwa sklepy markowe** (`kukirin-scooter.pl` i `kukirin.pl`) pozostają oddzielnie jako kanały dowodowe, bo ich rola handlowa i spójność danych wymagają ostrożności.

Wniosek: aliasy krytyczne dla shortlisty zostały znormalizowane wystarczająco do syntezy.

## 4. Kontrola final dispositions
Każdy wiersz w ledgerze ma teraz przypisane jedno z finalnych rozstrzygnięć:
- `conditional`
- `excluded`
- `unresolved`
- `benchmark-only`

Nie użyto `recommended`, bo raport ma pozostać szerokim accountingiem z sekcją rekomendacji warunkowych, a nie wąskim memo zakupowym.

Wniosek: **żaden śledzony byt nie znika bez rozstrzygnięcia**.

## 5. Główne konflikty i sposób rozstrzygnięcia
### Konflikt A — papierowy zasięg kontra zasięg niezależny
Rozstrzygnięcie:
- gdy istnieje mocny test Rider Guide, ma on wyższy priorytet niż karta sklepu,
- dlatego Nami Klima, Vsett 10+ i Teverun Fighter Mini Pro tracą siłę w kryterium „wiarygodny 60 km”, mimo mocnych deklaracji katalogowych.

### Konflikt B — świetna technika kontra słabsza pewność rynku PL
Rozstrzygnięcie:
- Kaabo Mantis Pro/10 Pro i część KuKirinów pozostają `unresolved`, gdy technika wygląda dobrze, ale aktywna i jednoznaczna oferta PL jest słabsza niż u Hiley/Techlife/Vsett/Teverun.

### Konflikt C — mocny serwis kontra słabsza „terenowość czysta”
Rozstrzygnięcie:
- Techlife Q7 RSX i Motus Pro 10 GT zostają `conditional`, bo przewagę budują stabilnością operacyjną i użytkową, niekoniecznie najbardziej terenowym profilem na papierze.

### Konflikt D — modele ważne rynkowo, ale zbyt słabe terenowo
Rozstrzygnięcie:
- Dualtron Popular, słabsze KuKiriny G2/G2 Pro i część niższych platform dostają `excluded`, bo nie wygrywają w priorytetach użytkownika.

## 6. Spójność shortlisty z raportem końcowym
Planowany raport ma uwzględnić jako głównych kandydatów / główne sekcje porównawcze:
- Hiley Tiger 10 V5 Performance
- Hiley Tiger 10 V5
- Teverun Fighter Mini Pro
- Techlife Q7 RSX V2
- KuKirin G2 Master
- KuKirin G4 Max
- Motus Pro 10 GT
- Vsett 10+ jako benchmark-only
- Nami Klima jako benchmark-only
- Kaabo Mantis Pro / 10 Pro jako unresolved

To jest spójne z ledgerem po finalnym domknięciu statusów.

## 7. Słabe miejsca, które pozostają jawnie oznaczone
- Brak pełnego niezależnego testu dla Hiley Tiger 10 V5 / Performance.
- Brak pełnego niezależnego testu zasięgu dla KuKirin G4 Max.
- Brak mocnego niezależnego testu terenowego dla Techlife Q7 RSX V2.
- Niejednoznaczność aktywnej polskiej sprzedaży konkretnej wersji Kaabo Mantis Pro / 10 Pro.
- Ograniczona twardość danych o awaryjności — sekcja niezawodności opiera się na proxy serwisowo-częściowych.

Wszystkie te luki pozostają w ledgerze i raporcie jako `conditional` lub `unresolved`, nie są ukrywane.

## 8. Werdykt reconciliation
Na potrzeby syntezy:
- wszystkie istotne encje mają wiersz w ledgerze,
- kluczowe aliasy są znormalizowane,
- każda istotna encja ma final disposition,
- słabe i konfliktowe dane są jawnie utrzymane jako `conditional`, `unresolved` lub `benchmark-only`,
- nie ma nierozliczonych luk pokrycia blokujących raport końcowy.

Reconciliation uznaje coverage za **wystarczająco domknięte do napisania raportu końcowego**.
