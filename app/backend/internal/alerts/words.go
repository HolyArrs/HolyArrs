package alerts

import "strings"

// words are the fixed parts of outbound alert messages in each app language,
// picked by the owner's saved language. A missing language or line falls back
// to English. {name} slots are filled by say.
var words = map[string]map[string]string{
	"en": {
		MomentGrab: "Sent to download", MomentImport: "In the Library", MomentRename: "Renamed", MomentAdd: "Added",
		MomentDelete: "Deleted", MomentDecision: "Needs a decision", MomentSubtitle: "Subtitle",
		"health_fixed": "Health: fixed", "health_look": "Health: something needs a look",
		"in_library":    "{name} is in the Library",
		"more_episodes": "{n} more episodes of this pack are in the Library:",
		"more_tracks":   "{n} more tracks of this album download are in the Library:",
		"episode_range": "{from} to {to} ({n} episodes)",
		"tracks":        "{n} tracks",
	},
	"es": {
		MomentGrab: "Enviado a descargar", MomentImport: "En la Biblioteca", MomentRename: "Renombrado", MomentAdd: "Añadido",
		MomentDelete: "Eliminado", MomentDecision: "Requiere una decisión", MomentSubtitle: "Subtítulo",
		"health_fixed": "Estado: resuelto", "health_look": "Estado: algo necesita atención",
		"in_library":    "{name} está en la Biblioteca",
		"more_episodes": "{n} episodios más de este paquete están en la Biblioteca:",
		"more_tracks":   "{n} pistas más de esta descarga de álbum están en la Biblioteca:",
		"episode_range": "{from} a {to} ({n} episodios)",
		"tracks":        "{n} pistas",
	},
	"pt-br": {
		MomentGrab: "Enviado para download", MomentImport: "Na Biblioteca", MomentRename: "Renomeado", MomentAdd: "Adicionado",
		MomentDelete: "Excluído", MomentDecision: "Precisa de uma decisão", MomentSubtitle: "Legenda",
		"health_fixed": "Saúde: resolvido", "health_look": "Saúde: algo precisa de atenção",
		"in_library":    "{name} está na Biblioteca",
		"more_episodes": "Mais {n} episódios deste pacote estão na Biblioteca:",
		"more_tracks":   "Mais {n} faixas deste download de álbum estão na Biblioteca:",
		"episode_range": "{from} a {to} ({n} episódios)",
		"tracks":        "{n} faixas",
	},
	"fr": {
		MomentGrab: "Envoyé au téléchargement", MomentImport: "Dans la bibliothèque", MomentRename: "Renommé", MomentAdd: "Ajouté",
		MomentDelete: "Supprimé", MomentDecision: "Décision requise", MomentSubtitle: "Sous-titre",
		"health_fixed": "État : réglé", "health_look": "État : quelque chose demande votre attention",
		"in_library":    "{name} est dans la bibliothèque",
		"more_episodes": "{n} épisodes de plus de ce pack sont dans la bibliothèque :",
		"more_tracks":   "{n} pistes de plus de ce téléchargement d'album sont dans la bibliothèque :",
		"episode_range": "{from} à {to} ({n} épisodes)",
		"tracks":        "{n} pistes",
	},
	"de": {
		MomentGrab: "Zum Download gesendet", MomentImport: "In der Bibliothek", MomentRename: "Umbenannt", MomentAdd: "Hinzugefügt",
		MomentDelete: "Gelöscht", MomentDecision: "Entscheidung nötig", MomentSubtitle: "Untertitel",
		"health_fixed": "Zustand: behoben", "health_look": "Zustand: etwas braucht einen Blick",
		"in_library":    "{name} ist in der Bibliothek",
		"more_episodes": "{n} weitere Folgen dieses Pakets sind in der Bibliothek:",
		"more_tracks":   "{n} weitere Tracks dieses Album-Downloads sind in der Bibliothek:",
		"episode_range": "{from} bis {to} ({n} Folgen)",
		"tracks":        "{n} Tracks",
	},
	"ru": {
		MomentGrab: "Отправлено на загрузку", MomentImport: "В Медиатеке", MomentRename: "Переименовано", MomentAdd: "Добавлено",
		MomentDelete: "Удалено", MomentDecision: "Требует решения", MomentSubtitle: "Субтитры",
		"health_fixed": "Состояние: исправлено", "health_look": "Состояние: нужно проверить",
		"in_library":    "{name} в Медиатеке",
		"more_episodes": "Ещё серий из этого пакета в Медиатеке: {n}",
		"more_tracks":   "Ещё треков из этой загрузки альбома в Медиатеке: {n}",
		"episode_range": "{from} – {to} (серий: {n})",
		"tracks":        "Треков: {n}",
	},
	"uk": {
		MomentGrab: "Надіслано на завантаження", MomentImport: "У Медіатеці", MomentRename: "Перейменовано", MomentAdd: "Додано",
		MomentDelete: "Видалено", MomentDecision: "Потрібне рішення", MomentSubtitle: "Субтитри",
		"health_fixed": "Стан: виправлено", "health_look": "Стан: треба перевірити",
		"in_library":    "{name} у Медіатеці",
		"more_episodes": "Ще серій із цього пакета в Медіатеці: {n}",
		"more_tracks":   "Ще треків із цього завантаження альбому в Медіатеці: {n}",
		"episode_range": "{from} – {to} (серій: {n})",
		"tracks":        "Треків: {n}",
	},
	"zh-cn": {
		MomentGrab: "已发送下载", MomentImport: "已入媒体库", MomentRename: "已重命名", MomentAdd: "已添加",
		MomentDelete: "已删除", MomentDecision: "待你决定", MomentSubtitle: "字幕",
		"health_fixed": "运行状况：已修复", "health_look": "运行状况：有问题需要查看",
		"in_library":    "{name} 已在媒体库中",
		"more_episodes": "此合集另有 {n} 集已在媒体库中：",
		"more_tracks":   "此专辑下载另有 {n} 首曲目已在媒体库中：",
		"episode_range": "{from} 至 {to}（{n} 集）",
		"tracks":        "{n} 首曲目",
	},
	"ja": {
		MomentGrab: "ダウンロードに送信", MomentImport: "ライブラリに追加済み", MomentRename: "名前を変更", MomentAdd: "追加",
		MomentDelete: "削除", MomentDecision: "判断待ち", MomentSubtitle: "字幕",
		"health_fixed": "状態：解決済み", "health_look": "状態：確認が必要です",
		"in_library":    "{name} がライブラリにあります",
		"more_episodes": "このパックのほか {n} 話がライブラリにあります：",
		"more_tracks":   "このアルバムダウンロードのほか {n} 曲がライブラリにあります：",
		"episode_range": "{from}〜{to}（{n} 話）",
		"tracks":        "{n} 曲",
	},
	"ko": {
		MomentGrab: "다운로드로 보냄", MomentImport: "라이브러리에 있음", MomentRename: "이름 바뀜", MomentAdd: "추가됨",
		MomentDelete: "삭제됨", MomentDecision: "결정 필요", MomentSubtitle: "자막",
		"health_fixed": "상태: 해결됨", "health_look": "상태: 확인이 필요해요",
		"in_library":    "{name}이(가) 라이브러리에 있어요",
		"more_episodes": "이 팩의 에피소드 {n}개가 더 라이브러리에 있어요:",
		"more_tracks":   "이 앨범 다운로드의 트랙 {n}개가 더 라이브러리에 있어요:",
		"episode_range": "{from}~{to} (에피소드 {n}개)",
		"tracks":        "트랙 {n}개",
	},
	"hi": {
		MomentGrab: "डाउनलोड के लिए भेजा", MomentImport: "लाइब्रेरी में", MomentRename: "नाम बदला", MomentAdd: "जोड़ा गया",
		MomentDelete: "हटाया गया", MomentDecision: "फ़ैसला चाहिए", MomentSubtitle: "सबटाइटल",
		"health_fixed": "स्थिति: ठीक हो गया", "health_look": "स्थिति: कुछ देखना ज़रूरी है",
		"in_library":    "{name} लाइब्रेरी में है",
		"more_episodes": "इस पैक के {n} और एपिसोड लाइब्रेरी में हैं:",
		"more_tracks":   "इस एल्बम डाउनलोड के {n} और ट्रैक लाइब्रेरी में हैं:",
		"episode_range": "{from} से {to} ({n} एपिसोड)",
		"tracks":        "{n} ट्रैक",
	},
	"it": {
		MomentGrab: "Inviato al download", MomentImport: "Nella Libreria", MomentRename: "Rinominato", MomentAdd: "Aggiunto",
		MomentDelete: "Eliminato", MomentDecision: "Serve una decisione", MomentSubtitle: "Sottotitolo",
		"health_fixed": "Salute: risolto", "health_look": "Salute: qualcosa va controllato",
		"in_library":    "{name} è nella Libreria",
		"more_episodes": "Altri {n} episodi di questo pacchetto sono nella Libreria:",
		"more_tracks":   "Altre {n} tracce di questo download dell'album sono nella Libreria:",
		"episode_range": "da {from} a {to} ({n} episodi)",
		"tracks":        "{n} tracce",
	},
	"nl": {
		MomentGrab: "Verstuurd om te downloaden", MomentImport: "In de Bibliotheek", MomentRename: "Hernoemd", MomentAdd: "Toegevoegd",
		MomentDelete: "Verwijderd", MomentDecision: "Beslissing nodig", MomentSubtitle: "Ondertitel",
		"health_fixed": "Gezondheid: opgelost", "health_look": "Gezondheid: er moet iets bekeken worden",
		"in_library":    "{name} staat in de Bibliotheek",
		"more_episodes": "Nog {n} afleveringen van dit pakket staan in de Bibliotheek:",
		"more_tracks":   "Nog {n} nummers van deze albumdownload staan in de Bibliotheek:",
		"episode_range": "{from} tot {to} ({n} afleveringen)",
		"tracks":        "{n} nummers",
	},
	"pl": {
		MomentGrab: "Wysłano do pobrania", MomentImport: "W Bibliotece", MomentRename: "Zmieniono nazwę", MomentAdd: "Dodano",
		MomentDelete: "Usunięto", MomentDecision: "Potrzebna decyzja", MomentSubtitle: "Napisy",
		"health_fixed": "Kondycja: naprawione", "health_look": "Kondycja: coś wymaga uwagi",
		"in_library":    "{name} jest w Bibliotece",
		"more_episodes": "Kolejne odcinki z tej paczki są w Bibliotece ({n}):",
		"more_tracks":   "Kolejne utwory z pobranego albumu są w Bibliotece ({n}):",
		"episode_range": "{from}–{to} (odcinków: {n})",
		"tracks":        "Utworów: {n}",
	},
	"tr": {
		MomentGrab: "İndirmeye gönderildi", MomentImport: "Kütüphanede", MomentRename: "Yeniden adlandırıldı", MomentAdd: "Eklendi",
		MomentDelete: "Silindi", MomentDecision: "Karar gerekiyor", MomentSubtitle: "Altyazı",
		"health_fixed": "Sağlık: düzeldi", "health_look": "Sağlık: bir şeye bakılması gerekiyor",
		"in_library":    "{name} Kütüphanede",
		"more_episodes": "Bu paketten {n} bölüm daha Kütüphanede:",
		"more_tracks":   "Bu albüm indirmesinden {n} parça daha Kütüphanede:",
		"episode_range": "{from} - {to} ({n} bölüm)",
		"tracks":        "{n} parça",
	},
	"ar": {
		MomentGrab: "أُرسل للتنزيل", MomentImport: "في المكتبة", MomentRename: "أُعيدت تسميته", MomentAdd: "أُضيف",
		MomentDelete: "حُذف", MomentDecision: "بحاجة إلى قرار", MomentSubtitle: "ترجمة",
		"health_fixed": "الحالة: تم الإصلاح", "health_look": "الحالة: شيء ما يحتاج إلى نظرة",
		"in_library":    "{name} في المكتبة",
		"more_episodes": "{n} حلقات أخرى من هذه الحزمة في المكتبة:",
		"more_tracks":   "{n} مقاطع أخرى من تنزيل هذا الألبوم في المكتبة:",
		"episode_range": "{from} إلى {to} ({n} حلقات)",
		"tracks":        "{n} مقاطع",
	},
}

// say is the line for key in lang (English when either is unknown), with
// {name} slots filled from pairs: "name", value, ...
func say(lang, key string, pairs ...string) string {
	line, ok := words[strings.ToLower(lang)][key]
	if !ok {
		line = words["en"][key]
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		line = strings.ReplaceAll(line, "{"+pairs[i]+"}", pairs[i+1])
	}
	return line
}
