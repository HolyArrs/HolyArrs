package main

import "strings"

// keyNotInConfig is the line the missing master key page adds, in each app
// language: where HolyArrs looked and what to check. Only fixed text, so
// the open page shows no file names or paths.
var keyNotInConfig = map[string]string{
	"ar":    "بحث HolyArrs في /config ولم يجد فيه أي مفتاح رئيسي. تحقّق من أن المجلد المربوط بـ /config ما زال يحتوي ملفات HolyArrs وأنه لم يُنقل.",
	"bg":    "HolyArrs потърси в /config и не намери главен ключ там. Проверете дали папката, свързана с /config, все още съдържа файловете на HolyArrs и не е преместена.",
	"bn":    "HolyArrs /config-এ খুঁজেছে, কিন্তু সেখানে কোনো মাস্টার কী পায়নি। দেখে নিন /config-এর সঙ্গে যুক্ত ফোল্ডারে এখনও HolyArrs-এর ফাইলগুলো আছে এবং সেটি সরানো হয়নি।",
	"ca":    "HolyArrs ha buscat a /config i no hi ha trobat cap clau mestra. Comprova que la carpeta assignada a /config encara conté els fitxers d'HolyArrs i que no s'ha mogut.",
	"cs":    "HolyArrs hledal v /config a nenašel tam hlavní klíč. Zkontrolujte, že složka připojená na /config stále obsahuje soubory HolyArrs a nebyla přesunuta.",
	"da":    "HolyArrs kiggede i /config og fandt ingen hovednøgle der. Tjek, at mappen, der er koblet til /config, stadig indeholder HolyArrs' filer og ikke er blevet flyttet.",
	"de":    "HolyArrs hat in /config nachgesehen und dort keinen Hauptschlüssel gefunden. Prüfe, ob der als /config eingebundene Ordner noch die Dateien von HolyArrs enthält und nicht verschoben wurde.",
	"el":    "Το HolyArrs έψαξε στο /config και δεν βρήκε εκεί κύριο κλειδί. Ελέγξτε ότι ο φάκελος που αντιστοιχεί στο /config περιέχει ακόμα τα αρχεία του HolyArrs και δεν έχει μετακινηθεί.",
	"en":    "HolyArrs looked in /config and found no master key there. Check that the folder mapped to /config still holds HolyArrs's files and was not moved.",
	"es":    "HolyArrs buscó en /config y no encontró ninguna clave maestra. Comprueba que la carpeta asignada a /config todavía tiene los archivos de HolyArrs y que no se movió.",
	"et":    "HolyArrs otsis kaustast /config ega leidnud sealt peavõtit. Kontrolli, et /config'iga seotud kaust sisaldab endiselt HolyArrsi faile ja seda pole teisaldatud.",
	"fa":    "HolyArrs در /config جستجو کرد و هیچ کلید اصلی‌ای آنجا پیدا نکرد. بررسی کنید که پوشه‌ای که به /config وصل است هنوز فایل‌های HolyArrs را دارد و جابه‌جا نشده است.",
	"fi":    "HolyArrs etsi kansiosta /config eikä löytänyt sieltä pääavainta. Tarkista, että /config-polkuun liitetty kansio sisältää yhä HolyArrsin tiedostot eikä sitä ole siirretty.",
	"fil":   "Tumingin ang HolyArrs sa /config at walang nakitang master key doon. Tiyaking nasa folder na naka-map sa /config pa rin ang mga file ng HolyArrs at hindi ito nailipat.",
	"fr":    "HolyArrs a cherché dans /config et n'y a trouvé aucune clé maîtresse. Vérifiez que le dossier associé à /config contient toujours les fichiers de HolyArrs et qu'il n'a pas été déplacé.",
	"he":    "HolyArrs חיפש ב־/config ולא מצא שם מפתח אב. בדוק שהתיקייה שממופה ל־/config עדיין מכילה את הקבצים של HolyArrs ושהיא לא הועברה.",
	"hi":    "HolyArrs ने /config में देखा, पर वहाँ कोई मास्टर कुंजी नहीं मिली। जांचें कि /config से जुड़े फ़ोल्डर में अब भी HolyArrs की फ़ाइलें हैं और उसे कहीं और नहीं ले जाया गया है।",
	"hr":    "HolyArrs je tražio u /config i ondje nije pronašao glavni ključ. Provjeri sadrži li mapa povezana s /config još uvijek datoteke HolyArrsa i da nije premještena.",
	"hu":    "A HolyArrs a /config mappában keresett, de nem talált ott főkulcsot. Ellenőrizze, hogy a /config-hoz csatolt mappa még tartalmazza-e a HolyArrs fájljait, és nincs-e áthelyezve.",
	"id":    "HolyArrs mencari di /config dan tidak menemukan kunci utama di sana. Pastikan folder yang dipetakan ke /config masih berisi file HolyArrs dan tidak dipindahkan.",
	"is":    "HolyArrs leitaði í /config og fann engan aðallykil þar. Athugaðu að mappan sem er tengd við /config innihaldi enn skrár HolyArrs og hafi ekki verið færð.",
	"it":    "HolyArrs ha cercato in /config e non ha trovato nessuna chiave principale. Controlla che la cartella collegata a /config contenga ancora i file di HolyArrs e che non sia stata spostata.",
	"ja":    "HolyArrs は /config を確認しましたが、マスターキーが見つかりませんでした。/config に割り当てたフォルダーに HolyArrs のファイルが残っていて、移動されていないか確認してください。",
	"ko":    "HolyArrs가 /config에서 찾아봤지만 마스터 키가 없었습니다. /config에 연결된 폴더에 HolyArrs 파일이 아직 있는지, 폴더가 옮겨지지 않았는지 확인하세요.",
	"lt":    "HolyArrs ieškojo /config ir ten nerado pagrindinio rakto. Patikrinkite, ar su /config susietame aplanke vis dar yra HolyArrs failai ir ar jis nebuvo perkeltas.",
	"lv":    "HolyArrs meklēja mapē /config un tur neatrada galveno atslēgu. Pārbaudi, vai mapē, kas piesaistīta /config, joprojām ir HolyArrs faili un vai tā nav pārvietota.",
	"ms":    "HolyArrs mencari dalam /config dan tidak menemui kunci induk di situ. Semak bahawa folder yang dipetakan ke /config masih menyimpan fail HolyArrs dan tidak dialihkan.",
	"nb":    "HolyArrs så i /config og fant ingen hovednøkkel der. Sjekk at mappen som er koblet til /config, fortsatt har filene til HolyArrs og ikke er flyttet.",
	"nl":    "HolyArrs heeft in /config gezocht en daar geen hoofdsleutel gevonden. Controleer of de map die aan /config is gekoppeld nog de bestanden van HolyArrs bevat en niet is verplaatst.",
	"pl":    "HolyArrs szukał w /config i nie znalazł tam klucza głównego. Sprawdź, czy folder podpięty pod /config nadal zawiera pliki HolyArrs i nie został przeniesiony.",
	"pt-br": "O HolyArrs procurou em /config e não encontrou nenhuma chave mestra. Confira se a pasta mapeada para /config ainda tem os arquivos do HolyArrs e não foi movida.",
	"pt-pt": "O HolyArrs procurou em /config e não encontrou nenhuma chave mestra. Verifique se a pasta mapeada para /config ainda tem os ficheiros do HolyArrs e não foi movida.",
	"ro":    "HolyArrs a căutat în /config și nu a găsit acolo nicio cheie principală. Verifică dacă folderul asociat cu /config conține încă fișierele HolyArrs și nu a fost mutat.",
	"ru":    "HolyArrs искал в /config и не нашёл там главный ключ. Проверьте, что папка, подключённая к /config, всё ещё содержит файлы HolyArrs и не была перемещена.",
	"sk":    "HolyArrs hľadal v /config a nenašiel tam hlavný kľúč. Skontrolujte, či priečinok pripojený k /config stále obsahuje súbory HolyArrs a nebol presunutý.",
	"sr":    "HolyArrs је тражио у /config и тамо није нашао главни кључ. Провери да ли фасцикла повезана са /config и даље садржи датотеке HolyArrs-а и да није премештена.",
	"sv":    "HolyArrs letade i /config och hittade ingen huvudnyckel där. Kontrollera att mappen som är kopplad till /config fortfarande innehåller HolyArrs filer och inte har flyttats.",
	"ta":    "HolyArrs /config-இல் தேடியது, ஆனால் அங்கு முதன்மை விசை இல்லை. /config-உடன் இணைக்கப்பட்ட கோப்புறையில் HolyArrs கோப்புகள் இன்னும் உள்ளனவா, அது நகர்த்தப்படவில்லையா என்று சரிபார்க்கவும்.",
	"th":    "HolyArrs ค้นหาใน /config แล้วแต่ไม่พบคีย์หลักที่นั่น ตรวจสอบว่าโฟลเดอร์ที่แมปกับ /config ยังมีไฟล์ของ HolyArrs อยู่และไม่ได้ถูกย้ายไปที่อื่น",
	"tr":    "HolyArrs /config içinde aradı ama orada ana anahtar bulamadı. /config'e bağlı klasörün hâlâ HolyArrs dosyalarını içerdiğini ve taşınmadığını kontrol edin.",
	"uk":    "HolyArrs шукав у /config і не знайшов там головного ключа. Перевірте, що папка, підключена до /config, досі містить файли HolyArrs і не була переміщена.",
	"vi":    "HolyArrs đã tìm trong /config nhưng không thấy khóa chính nào. Hãy kiểm tra thư mục được gắn vào /config vẫn còn các tệp của HolyArrs và chưa bị di chuyển.",
	"zh-cn": "HolyArrs 在 /config 中查找过，但没有找到主密钥。请检查映射到 /config 的文件夹是否仍保存着 HolyArrs 的文件，且没有被移走。",
	"zh-tw": "HolyArrs 在 /config 中找過，但沒有找到主金鑰。請檢查對應到 /config 的資料夾是否仍保存著 HolyArrs 的檔案，且沒有被移走。",
}

// rightToLeft are the app languages written right to left.
var rightToLeft = map[string]bool{"ar": true, "fa": true, "he": true}

// keyLine is the /config line for an app language, English when there is
// none. The page is plain text, which browsers lay out left to right, so a
// right-to-left line goes in a right-to-left isolate to keep its pieces in
// order.
func keyLine(lang string) string {
	line, ok := keyNotInConfig[lang]
	if !ok {
		return keyNotInConfig["en"]
	}
	if rightToLeft[lang] {
		// The path keeps its own left-to-right order inside the sentence.
		line = strings.ReplaceAll(line, "/config", "⁦/config⁩")
		return "⁧" + line + "⁩"
	}
	return line
}
