package alpha

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReindexLangTerm(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @lang .`))

	m1 := `{
    set {
      _:u1 <name> "Ab Bc"@en .
      _:u2 <name> "Bc Cd"@en .
      _:u3 <name> "Cd Da"@fr .
    }
  }`
	_, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// perform re-indexing
	require.NoError(t, alterSchema(`name: string @lang @index(term) .`))

	q1 := `{
      q(func: anyofterms(name@en, "bc")) {
        name@en
      }
    }`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"q":[{"name@en":"Ab Bc"},{"name@en":"Bc Cd"}]}}`, res)
}

func TestReindexData(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @lang .`))

	m1 := `{
    set {
      <10000> <name>	"Vidar Flataukan"@en	.
      <10001> <name>	"starring"@en	.
      <10002> <name>	"cinematography"@en	.
      <10003> <name>	"directed_by"@en	.
      <10004> <name>	"edited_by"@en	.
      <10005> <name>	"festivals"@en	.
      <10006> <name>	"genre"@en	.
      <10007> <name>	"Movies"@en	.
      <10008> <name>	"music"@en	.
      <10009> <name>	"produced_by"@en	.
      <10010> <name>	"production_companies"@en	.
      <10011> <name>	"written_by"@en	.
      <10012> <name>	"Films edited"@en	.
      <10013> <name>	"Apple movie trailer ID"@en	.
      <10014> <name>	"Films art directed"@en	.
      <10015> <name>	"Films casting directed"@en	.
      <10016> <name>	"Portrayed in films (dubbed)"@en	.
      <10017> <name>	"Portrayed in films"@en	.
      <10018> <name>	"Cinematography"@en	.
      <10019> <name>	"Films In Collection"@en	.
      <10020> <name>	"Films"@en	.
      <10021> <name>	"Companies performing this role or service"@en	.
      <10022> <name>	"Costume design by"@en	.
      <10023> <name>	"Costume Design for Film"@en	.
      <10024> <name>	"Country of origin"@en	.
      <10025> <name>	"Crewmember"@en	.
      <10026> <name>	"Film crew role"@en	.
      <10027> <name>	"Film"@en	.
      <10028> <name>	"Films crewed"@en	.
      <10029> <name>	"Film release region"@en	.
      <10030> <name>	"Film"@en	.
      <10031> <name>	"Note"@en	.
      <10032> <name>	"Runtime"@en	.
      <10033> <name>	"Film cut"@en	.
      <10034> <name>	"Films distributed in this medium"@en	.
      <10035> <name>	"Films distributed"@en	.
      <10036> <name>	"Distributors"@en	.
      <10037> <name>	"Dubbing performances"@en	.
      <10038> <name>	"Edited by"@en	.
      <10039> <name>	"Estimated budget"@en	.
      <10040> <name>	"Executive produced by"@en	.
      <10041> <name>	"Fandango ID"@en	.
      <10042> <name>	"Notable filming locations"@en	.
      <10043> <name>	"Featured in film"@en	.
      <10044> <name>	"Performed by"@en	.
      <10045> <name>	"Featured Song"@en	.
      <10046> <name>	"Date founded"@en	.
      <10047> <name>	"Closing date"@en	.
      <10048> <name>	"Festival"@en	.
      <10049> <name>	"Films"@en	.
      <10050> <name>	"Opening date"@en	.
      <10051> <name>	"Venues"@en	.
      <10052> <name>	"Festivals with this focus"@en	.
      <10053> <name>	"Focus"@en	.
      <10054> <name>	"Individual festivals"@en	.
      <10055> <name>	"Location"@en	.
      <10056> <name>	"Festivals sponsored"@en	.
      <10057> <name>	"Sponsoring organization"@en	.
      <10058> <name>	"Festival"@en	.
      <10059> <name>	"From"@en	.
      <10060> <name>	"Sponsor"@en	.
      <10061> <name>	"To"@en	.
      <10062> <name>	"Art direction by"@en	.
      <10063> <name>	"Casting director"@en	.
      <10064> <name>	"Film Collections"@en	.
      <10065> <name>	"Film company"@en	.
      <10066> <name>	"Film cut"@en	.
      <10067> <name>	"Film"@en	.
      <10068> <name>	"Role/service"@en	.
      <10069> <name>	"Distributor"@en	.
      <10070> <name>	"Film cut"@en	.
      <10071> <name>	"Film distribution medium"@en	.
      <10072> <name>	"Film"@en	.
      <10073> <name>	"Region"@en	.
      <10074> <name>	"Year"@en	.
      <10075> <name>	"Film festivals"@en	.
      <10076> <name>	"Film format"@en	.
      <10077> <name>	"Filming"@en	.
      <10078> <name>	"Production design by"@en	.
      <10079> <name>	"Film Series"@en	.
      <10080> <name>	"Set Decoration by"@en	.
      <10081> <name>	"Film Format"@en	.
      <10082> <name>	"Genres"@en	.
      <10083> <name>	"Gross revenue"@en	.
      <10084> <name>	"Initial release date"@en	.
      <10085> <name>	"Films with this crew job"@en	.
      <10086> <name>	"Languages"@en	.
      <10087> <name>	"Featured In Films"@en	.
      <10088> <name>	"Featured Locations"@en	.
      <10089> <name>	"Metacritic film ID"@en	.
      <10090> <name>	"Music by"@en	.
      <10091> <name>	"Netflix ID"@en	.
      <10092> <name>	"NY Times ID"@en	.
      <10093> <name>	"Other crew"@en	.
      <10094> <name>	"Other film companies"@en	.
      <10095> <name>	"Personal appearances"@en	.
      <10096> <name>	"Post-production"@en	.
      <10097> <name>	"Pre-production"@en	.
      <10098> <name>	"Prequel"@en	.
      <10099> <name>	"Primary language"@en	.
      <10100> <name>	"Produced by"@en	.
      <10101> <name>	"Production companies"@en	.
      <10102> <name>	"Films production designed"@en	.
      <10103> <name>	"Rated"@en	.
      <10104> <name>	"Film regional debut venue"@en	.
      <10105> <name>	"Film release distribution medium"@en	.
      <10106> <name>	"Film release region"@en	.
      <10107> <name>	"Film"@en	.
      <10108> <name>	"Release Date"@en	.
      <10109> <name>	"Release date(s)"@en	.
      <10110> <name>	"Rotten Tomatoes ID"@en	.
      <10111> <name>	"Runtime"@en	.
      <10112> <name>	"Sequel"@en	.
      <10113> <name>	"Films In Series"@en	.
      <10114> <name>	"Film sets designed"@en	.
      <10115> <name>	"Films"@en	.
      <10116> <name>	"Film songs"@en	.
      <10117> <name>	"Composition"@en	.
      <10118> <name>	"Film"@en	.
      <10119> <name>	"Performers"@en	.
      <10120> <name>	"Songs"@en	.
      <10121> <name>	"Soundtrack"@en	.
      <10122> <name>	"Performances"@en	.
      <10123> <name>	"Story by"@en	.
      <10124> <name>	"Film Story Credits"@en	.
      <10125> <name>	"Films On This Subject"@en	.
      <10126> <name>	"Subjects"@en	.
      <10127> <name>	"Tagline"@en	.
      <10128> <name>	"Trailer Addict ID"@en	.
      <10129> <name>	"Trailers"@en	.
      <10130> <name>	"Screenplay by"@en	.
      <10131> <name>	"Film music credits"@en	.
      <10132> <name>	"Actor"@en	.
      <10133> <name>	"Character note"@en	.
      <10134> <name>	"Character"@en	.
      <10135> <name>	"Film"@en	.
      <10136> <name>	"Special Performance Type"@en	.
      <10137> <name>	"Film"@en	.
      <10138> <name>	"Person"@en	.
      <10139> <name>	"Film appearances"@en	.
      <10140> <name>	"Type of Appearance"@en	.
      <10141> <name>	"Films appeared in"@en	.
      <10142> <name>	"Films Executive Produced"@en	.
      <10143> <name>	"Films Produced"@en	.
      <10144> <name>	"Films"@en	.
      <10145> <name>	"Special Film Performance"@en	.
      <10146> <name>	"Film writing credits"@en	.
      <10147> <name>	"What Price Goofy"@en	.
      <10148> <name>	"Vlasta Zehrova"@en	.
      <10149> <name>	"General Orlov"@en	.
      <10150> <name>	"Corinne Dufour"@en	.
      <10151> <name>	"Sunshine and Gold"@en	.
      <10152> <name>	"Moon Over Tao"@en	.
      <10153> <name>	"Pasquale Marino"@en	.
      <10154> <name>	"Matteo Capelli"@en	.
      <10155> <name>	"Michal Hejný"@en	.
      <10156> <name>	"Karioka"@en	.
      <10157> <name>	"Yossi Eini"@en	.
      <10158> <name>	"Viktor Gordeyev"@en	.
      <10159> <name>	"Roos Van Vlaenderen"@en	.
      <10160> <name>	"General Georgi Koskov"@en	.
      <10161> <name>	"Do kalle-shagh"@en	.
      <10162> <name>	"Moataz El Tony"@en	.
      <10163> <name>	"Dirty Dreamer"@en	.
      <10164> <name>	"Karl Sturm"@en	.
      <10165> <name>	"Michael Ehninger"@en	.
      <10166> <name>	"Killer of Small Fishes"@en	.
      <10167> <name>	"Jean-Pierre Dougnac"@en	.
      <10168> <name>	"Yes We Can!"@en	.
      <10169> <name>	"Jaroslaw Dunaj"@en	.
      <10170> <name>	"William O'Farrell"@en	.
      <10171> <name>	"Melodies, Melodies..."@en	.
      <10172> <name>	"Jerzy Sagan"@en	.
      <10173> <name>	"Only the Valiant"@en	.
      <10174> <name>	"Socarrat"@en	.
      <10175> <name>	"Yuet-Ming Yiu"@en	.
      <10176> <name>	"Georg Wratsch"@en	.
      <10177> <name>	"Zivorad Zika Pavlovic"@en	.
      <10178> <name>	"Shinsuke Akagi"@en	.
      <10179> <name>	"The Donkey Who Drank the Moon"@en	.
      <10180> <name>	"Tanemaku tabibito: Minori no cha"@en	.
      <10181> <name>	"The Crossing"@en	.
      <10182> <name>	"Samvel Muzhikyan"@en	.
      <10183> <name>	"Breaking the Wall"@en	.
      <10184> <name>	"Tommaso Blu"@en	.
      <10185> <name>	"Mário Viegas"@en	.
      <10186> <name>	"Carl Struve"@en	.
      <10187> <name>	"Pine Tree"@en	.
      <10188> <name>	"Lost Youth"@en	.
      <10189> <name>	"Pasi"@en	.
      <10190> <name>	"Grigori Kirillov"@en	.
      <10191> <name>	"Valentina Kutsenko"@en	.
      <10192> <name>	"Fleeing by Night"@en	.
      <10193> <name>	"Salwa Nakkara"@en	.
      <10194> <name>	"Li Wei Chang"@en	.
      <10195> <name>	"Plenty O'Toole"@en	.
      <10196> <name>	"Thomás Tristonho"@en	.
      <10197> <name>	"Sjamsuddin Jusuf"@en	.
      <10198> <name>	"Tit for Tat"@en	.
      <10199> <name>	"Svetlana Korkoshko"@en	.
      <10200> <name>	"Badiaa Masabny"@en	.
      <10201> <name>	"Edith Toreg"@en	.
      <10202> <name>	"Casey Alexander"@en	.
      <10203> <name>	"Xie Yuan"@en	.
      <10204> <name>	"Feliks Rybicki"@en	.
      <10205> <name>	"Early Rain"@en	.
      <10206> <name>	"The Virgin Star"@en	.
      <10207> <name>	"When the Cloud Scatters Away"@en	.
      <10208> <name>	"Helen Ward"@en	.
      <10209> <name>	"Yoo Se-Hung"@en	.
      <10210> <name>	"Dancing with Father"@en	.
      <10211> <name>	"Joana Nin"@en	.
      <10212> <name>	"A Tearstained Crown"@en	.
      <10213> <name>	"Yael Reuveny"@en	.
      <10214> <name>	"Merrick"@en	.
      <10215> <name>	"Teuda Bara"@en	.
      <10216> <name>	"Chris Ayers"@en	.
      <10217> <name>	"Trust, at least trust"@en	.
      <10218> <name>	"Rerberg and Tarkovsky - Reverse Side of \"Stalker\""@en	.
      <10219> <name>	"Stefan Forss"@en	.
      <10220> <name>	"Myeongdong vs Nampodong"@en	.
      <10221> <name>	"A Daughter of a Condemned Criminal"@en	.
      <10222> <name>	"Alone Crying Star"@en	.
      <10223> <name>	"Though the Heavens May Fall"@en	.
      <10224> <name>	"Friendship of Hope"@en	.
      <10225> <name>	"Hedpoh Chernyim"@en	.
      <10226> <name>	"Cheng-Peng Kao"@en	.
      <10227> <name>	"Yin Hang"@en	.
      <10228> <name>	"脱走遊戯"@ja	.
      <10229> <name>	"まむしと青大将"@ja	.
      <10230> <name>	"網走番外地 吹雪の斗争"@ja	.
      <10231> <name>	"結婚って、幸せですか THE MOVIE"@ja	.
      <10232> <name>	"スープ〜生まれ変わりの物語〜"@ja	.
      <10233> <name>	"ドミニク・グリーン"@ja	.
      <10234> <name>	"コント55号 世紀の大弱点"@ja	.
      <10152> <name>	"タオの月"@ja	.
      <10178> <name>	"赤木伸輔"@ja	.
      <10180> <name>	"種まく旅人～みのりの茶～"@ja	.
      <10235> <name>	"Лев Любецкий"@ru	.
      <10236> <name>	"Смерть Таирова"@ru	.
      <10237> <name>	"Михаль Рейно"@ru	.
      <10238> <name>	"Фрэнк Бёрнс"@ru	.
      <10239> <name>	"Ловец Макинтайр"@ru	.
      <10240> <name>	"Хавьер Перес Гробет"@ru	.
      <10241> <name>	"Джулио Пампильоне"@ru	.
      <10242> <name>	"Теодора Ремундова"@ru	.
      <10243> <name>	"Дин Каримович Махаматдинов"@ru	.
      <10158> <name>	"Виктор Владимирович Гордеев"@ru	.
      <10244> <name>	"Ирина Ивановна Поплавская"@ru	.
      <10202> <name>	"Кэйси Александр"@ru	.
      <10245> <name>	"Артём Витальевич Цыпин"@ru	.
      <10246> <name>	"بياعة الجرايد"@ar	.
      <10247> <name>	"شفيقة القبطية"@ar	.
      <10156> <name>	"كاريوكا"@ar	.
      <10200> <name>	"بديعة مصابنى"@ar	.
      <10248> <name>	"手機裡的眼淚"@zh-Hant	.
      <10249> <name>	"먼동이 틀 때"@ko	.
      <10250> <name>	"사랑과 헤이세이의 색남"@ko	.
      <10251> <name>	"9. Altın Koza Film Festivali"@tr	.
      <10252> <name>	"아쉬칸 카티비"@ko	.
      <10253> <name>	"Charles Kawalsky"@fr	.
      <10254> <name>	"박철호"@ko	.
      <10255> <name>	"Ruggero Cara"@ca	.
      <10240> <name>	"Xavier Pérez Grobet"@es-419	.
      <10240> <name>	"Xavier Pérez Grobet"@fr	.
      <10240> <name>	"Xavier Pérez Grobet"@sl	.
      <10256> <name>	"최옥희"@ko	.
      <10257> <name>	"George Wang"@de	.
      <10257> <name>	"George Wang"@es-419	.
      <10257> <name>	"王玨"@zh	.
      <10257> <name>	"王玨"@zh-Hant	.
      <10258> <name>	"Danie miłości"@pl	.
      <10258> <name>	"Joyful Reunion"@ro	.
      <10258> <name>	"飲食男女：好遠又好近"@zh-Hant	.
      <10259> <name>	"Kim Kristensen"@da	.
      <10259> <name>	"Kim Kristensen"@nl	.
      <10260> <name>	"Ecaterina Teodoroiu"@ro	.
      <10261> <name>	"써가"@ko	.
      <10262> <name>	"12. Altın Koza Film Festivali"@tr	.
      <10263> <name>	"5. Altın Koza Film Festivali"@tr	.
      <10264> <name>	"선택"@ko	.
      <10265> <name>	"ناصر مهدی‌پور"@fa	.
      <10266> <name>	"Rita Elmgren"@sv	.
      <10267> <name>	"Intruso"@pt-PT	.
      <10268> <name>	"서장원"@ko	.
      <10150> <name>	"Corinne Dufour"@fr	.
      <10159> <name>	"Roos Van Vlaenderen"@fr	.
      <10159> <name>	"Roos Van Vlaenderen"@pl	.
      <10159> <name>	"Roos Van Vlaenderen"@ro	.
      <10159> <name>	"Roos Van Vlaenderen"@tr	.
      <10160> <name>	"General Koskov"@sv	.
      <10269> <name>	"Karolina Kaiser"@hu	.
      <10270> <name>	"朱芷瑩"@zh-Hant	.
      <10271> <name>	"Sune Spangberg"@ro	.
      <10271> <name>	"Sune Spångberg"@sv	.
      <10272> <name>	"Utena a garota revolucionaria : Uma aventura Mágica"@pt	.
      <10272> <name>	"Utena a garota revolucionaria : Uma aventura Mágica"@pt-BR	.
      <10273> <name>	"Irja Elstelä"@fi	.
      <10171> <name>	"Melodii, melodii"@ro	.
      <10172> <name>	"Jerzy Sagan"@pl	.
      <10173> <name>	"La carga de los valientes"@es	.
      <10177> <name>	"Живорад Жика Павловић"@sr	.
      <10181> <name>	"La Traversée"@fr	.
      <10181> <name>	"La traversée"@tr	.
      <10182> <name>	"Samvel Muzhikyan"@de	.
      <10182> <name>	"Samvel Muzhikyan"@pl	.
      <10183> <name>	"성벽을 뚫고"@ko	.
      <10185> <name>	"Mário Viegas"@pt	.
      <10187> <name>	"일송정 푸른 솔은"@ko	.
      <10188> <name>	"잃어버린 청춘"@ko	.
      <10189> <name>	"파시"@ko	.
      <10192> <name>	"야분"@ko	.
      <10193> <name>	"Salwa Nakkara"@fr	.
      <10193> <name>	"Salwa Nakkara"@nl	.
      <10193> <name>	"Salwa Nakkara"@ro	.
      <10274> <name>	"애국자의 아들"@ko	.
      <10275> <name>	"Inês de Portugal"@pt-PT	.
      <10202> <name>	"Casey Alexander"@de	.
      <10203> <name>	"謝園"@zh-Hant	.
      <10204> <name>	"Feliks Rybicki"@pl	.
      <10205> <name>	"초우"@ko	.
      <10206> <name>	"처녀 별"@ko	.
      <10207> <name>	"구름이 흩어질 때"@ko	.
      <10210> <name>	"아빠와 함께 춤을"@ko	.
      <10211> <name>	"Joana Nin"@pt	.
      <10212> <name>	"눈물젖은 왕관"@ko	.
      <10213> <name>	"Yael Reuveny"@sl	.
      <10276> <name>	"이별의 종착역"@ko	.
      <10277> <name>	"아내를 빼앗긴 사나이"@ko	.
      <10278> <name>	"울지 않으련다"@ko	.
      <10215> <name>	"Teuda Bara"@pt	.
      <10215> <name>	"Teuda Bara"@ro	.
      <10215> <name>	"Teuda Bara"@tr	.
      <10217> <name>	"없어도 의리만은"@ko	.
      <10218> <name>	"Rerberg i Tarkowski. Odwrotna strona Stalkera"@pl	.
      <10219> <name>	"Stefan Forss"@fi	.
      <10220> <name>	"명동 사나이와 남포동 사나이"@ko	.
      <10221> <name>	"사형수의 딸"@ko	.
      <10222> <name>	"홀로 우는 별"@ko	.
      <10279> <name>	"내 청춘에 한은 없다"@ko	.
      <10223> <name>	"하늘이 무너져도"@ko	.
      <10224> <name>	"내일 있는 우정"@ko	.
      <10280> <name>	"젊은 그들"@ko	.
      <10225> <name>	"เห็ดเผาะ เชิญยิ้ม"@th	.
      <10281> <name>	"젊은 설계도"@ko	.
      <10226> <name>	"高振鵬"@zh-Hant	.
      <10282> <name>	"젊은 아들의 마지막 노래"@ko	.
      <10227> <name>	"尹航"@zh-Hant	.
      <10283> <name>	"Mahmood Sammakbashi"@de	.
      <10284> <name>	"Gárdonyi Lajos"@hu	.
      <10285> <name>	"아들의 심판"@ko	.
      <10286> <name>	"목숨걸고 왔수다"@ko	.
      <10287> <name>	"항구의 일야"@ko	.
      <10288> <name>	"남포동 출신"@ko	.
      <10289> <name>	"체스트"@ko	.
      <10289> <name>	"Це не я, це — він"@uk	.
    }
  }`
	_, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// reindex
	require.NoError(t, alterSchema(`name: string @lang @index(exact) .`))

	q1 := `{
    q(func: eq(name@en, "Runtime")) {
      uid
      name@en
    }
  }`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x2730",
          "name@en": "Runtime"
        },
        {
          "uid": "0x277f",
          "name@en": "Runtime"
        }
      ]
    }
  }`, res)

	// adding another triplet
	m2 := `{ set { <10400> <name>	"Runtime"@en	. }}`
	_, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x2730",
          "name@en": "Runtime"
        },
        {
          "uid": "0x277f",
          "name@en": "Runtime"
        },
        {
          "uid": "0x28a0",
          "name@en": "Runtime"
        }
      ]
    }
  }`, res)
}
