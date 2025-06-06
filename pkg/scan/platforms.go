package scan

import (
	"strings"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type Platform string

const (
	PlatformJava   Platform = "java"
	PlatformPhp    Platform = "php"
	PlatformNode   Platform = "node"
	PlatformPython Platform = "python"
	PlatformRuby   Platform = "ruby"
	PlatformGo     Platform = "go"
	PlatformAsp    Platform = "asp"
	PlatformPerl   Platform = "perl"
)

func ParsePlatform(platform string) Platform {
	switch strings.ToLower(platform) {
	case "java":
		return PlatformJava
	case "php":
		return PlatformPhp
	case "node":
		return PlatformNode
	case "python":
		return PlatformPython
	case "ruby":
		return PlatformRuby
	case "go":
		return PlatformGo
	case "aspnet":
		return PlatformAsp
	default:
		return ""
	}
}

func (p Platform) String() string {
	return string(p)
}

func (p Platform) MatchesAnyFingerprint(fingerprints []lib.Fingerprint) bool {
	if len(fingerprints) == 0 {
		return false
	}
	for _, fingerprint := range fingerprints {
		for _, software := range SoftwareList(p) {
			if strings.EqualFold(fingerprint.Name, software) {
				log.Info().Str("platform", p.String()).Str("fingerprint", fingerprint.Name).Msg("Matched fingerprint with a platform")
				return true
			}
		}
	}
	log.Debug().Str("platform", p.String()).Interface("fingerprints", fingerprints).Msg("No matching fingerprints found for platform")
	return false

}

func SoftwareList(platform Platform) []string {
	switch platform {
	case PlatformJava:
		return JavaSoftwareList()
	case PlatformPhp:
		return PhpSoftwareList()
	case PlatformNode:
		return NodeSoftwareList()
	case PlatformPython:
		return PythonSoftwareList()
	case PlatformRuby:
		return RubySoftwareList()
	case PlatformGo:
		return GoSoftwareList()
	case PlatformAsp:
		return AspNetSoftwareList()
	case PlatformPerl:
		return PerlSoftwareList()
	default:
		return []string{}
	}
}

func PythonSoftwareList() []string {
	return []string{
		"python",
		"django",
		"flask",
		"pyramid",
		"bottle",
		"web2py",
		"tornado",
		"fastapi",
		"falcon",
		"dash",
		"streamlit",
		"pylons",
		"cherrypy",
		"uvicorn",
		"gunicorn",
	}
}

func RubySoftwareList() []string {
	return []string{
		"ruby",
		"ruby on rails",
		"sinatra",
		"padrino",
		"hanami",
		"cuba",
		"ramaze",
		"roda",
		"volt",
		"coco",
		"kamal",
	}
}

func GoSoftwareList() []string {
	return []string{
		"go",
		"gin",
		"echo",
		"beego",
		"revel",
		"fiber",
		"chi",
		"gofiber",
		"iris",
		"echo framework",
	}
}

func AspNetSoftwareList() []string {
	return []string{
		"asp",
		"asp.net",
		"asp.net core",
		"dotnet",
		"microsoft asp.net",
		"microsoft dotnet",
		"microsoft .net",
		"microsoft .net core",
		"microsoft web api",
		"microsoft mvc",
	}
}

func PerlSoftwareList() []string {
	return []string{
		"perl",
		"catalyst",
		"dancer",
		"mojo",
		"plack",
		"mason",
		"template toolkit",
		"cgi.pm",
		"mod_perl",
	}
}

func JavaSoftwareList() []string {
	return []string{
		"java",
		"Adobe Experience Manager",
		"Ametys",
		"Apache Tomcat",
		"Apache Wicket",
		"Apereo CAS",
		"Atlassian Confluence",
		"Atlassian Jira",
		"Blade",
		"Brightspot",
		"Ckan",
		"Contensis",
		"DM Polopoly",
		"Gerrit",
		"Gitiles",
		"GlassFish",
		"Google Web Toolkit",
		"HCL Commerce",
		"HCL Digital Experience",
		"HCL Domino",
		"Halo",
		"JAlbum",
		"Java Servlet",
		"JavaServer Faces",
		"JavaServer Pages",
		"Jenkins",
		"Jetty",
		"K-Sup",
		"Liferay",
		"Lucene",
		"Open-Xchange App Suite",
		"OpenCms",
		"OpenGSE",
		"OpenGrok",
		"Oracle WebLogic Server",
		"Public CMS",
		"Resin",
		"SAP Commerce Cloud",
		"Skolengo",
		"SonarQubes",
		"Spring",
		"TeamCity",
		"Vaadin",
		"ZK",
		"Zimbra",
		"pirobase CMS",
		"uPortal",
	}
}

func PhpSoftwareList() []string {
	return []string{
		"php",
		"1C-Bitrix",
		"AbhiCMS",
		"Adminer",
		"Aegea",
		"Aksara CMS",
		"AlvandCMS",
		"Amiro.CMS",
		"Apereo CAS",
		"Arastta",
		"BIGACE",
		"Backdrop",
		"Banshee",
		"Batflat",
		"BigTree CMS",
		"Bigware",
		"BoidCMS",
		"Bolt CMS",
		"BookStack",
		"Brownie",
		"CMS Made Simple",
		"CMSimple",
		"CPG Dragonfly",
		"CS Cart",
		"Cachet",
		"CakePHP",
		"Cargo",
		"Centminmod",
		"Chamilo",
		"Chevereto",
		"Classeh",
		"ClickHeat",
		"Clockwork",
		"Cloudify.store",
		"Cloudrexx",
		"CodeIgniter",
		"Concrete CMS",
		"Contao",
		"Contenido",
		"Convertr",
		"Coppermine",
		"Cotonti",
		"CubeCart",
		"Danneo CMS",
		"DataLife Engine",
		"DedeCMS",
		"DirectAdmin",
		"Discuz! X",
		"Dokeos",
		"DokuWiki",
		"Dotclear",
		"Dotser",
		"Drupal",
		"EC-CUBE",
		"Ebasnet",
		"ElasticSuite",
		"Elcodi",
		"Eleanor CMS",
		"Eticex",
		"Eveve",
		"ExpressionEngine",
		"FUDforum",
		"Fat-Free Framework",
		"Flarum",
		"FluxBB",
		"Flyspray",
		"GLPI",
		"Gambio",
		"GetSimple CMS",
		"Gnuboard",
		"Grav",
		"Hamechio",
		"HeliumWeb",
		"Hotaru CMS",
		"Huberway",
		"IPB",
		"Ibexa DXP ",
		"ImpressCMS",
		"ImpressPages",
		"Indexhibit",
		"InstantCMS",
		"JobberBase",
		"Joomla",
		"KPHP",
		"Kitcart",
		"Koala Framework",
		"Kohana",
		"Koken",
		"Komodo CMS",
		"Kooomo",
		"LEPTON",
		"Laravel",
		"LightMon Engine",
		"Lithium",
		"LiveStreet CMS",
		"MODX",
		"Magento",
		"MaxSite CMS",
		"MaxenceDEVCMS",
		"MediaWiki",
		"Melis Platform",
		"Moguta.CMS",
		"Moodle",
		"MotoCMS",
		"MyBB",
		"Neos Flow",
		"Nette Framework",
		"Nextcloud",
		"NexusPHP",
		"OXID eShop",
		"OXID eShop Community Edition",
		"OXID eShop Enterprise Edition",
		"OXID eShop Professional Edition",
		"Omurga Sistemi",
		"OnShop",
		"Open Journal Systems",
		"Open eShop",
		"OpenCart",
		"OpenElement",
		"OpenSwoole",
		"OroCommerce",
		"PHP-Nuke",
		"PHPFusion",
		"Pantheon",
		"Paymenter",
		"Phabricator",
		"PhotoShelter",
		"Pimcore",
		"Pingoteam",
		"PrestaShop",
		"ProcessWire",
		"Proximis Unified Commerce",
		"Pterodactyl Panel",
		"Question2Answer",
		"RBS Change",
		"REDAXO",
		"RainLoop",
		"RiteCMS",
		"RoadRunner",
		"Roadiz CMS",
		"RoundCube",
		"Rubedo",
		"SPIP",
		"SQL Buddy",
		"Saly",
		"Sapren",
		"Seko OmniReturns",
		"Selldone",
		"Serendipity",
		"Shopery",
		"Shoptet",
		"Shopware",
		"Shuttle",
		"Silverstripe",
		"Simple Machines Forum",
		"SimpleSAMLphp",
		"SitePad",
		"Skilldo",
		"Sky-Shop",
		"Solodev",
		"SoteShop",
		"SpiritShop",
		"SquirrelMail",
		"Squiz Matrix",
		"Statamic",
		"Subrion",
		"SummerCart",
		"Symfony",
		"TYPO3 CMS",
		"Tebex",
		"Textpattern CMS",
		"Thelia",
		"ThinkPHP",
		"TwistPHP",
		"Typecho",
		"UMI.CMS",
		"Ultimate Bulletin Board",
		"Upvoty",
		"Ushahidi",
		"Uvodo",
		"Vanilla",
		"Visual Composer",
		"Webasyst Shop-Script",
		"Weblication",
		"Website Creator",
		"WebsiteBaker",
		"Weebly",
		"Wolf CMS",
		"Woltlab Community Framework",
		"WordPress",
		"X-Cart",
		"XAMPP",
		"XOOPS",
		"XenForo",
		"Yii",
		"Yoori",
		"YouCan",
		"Zabbix",
		"Zoey",
		"Zozo",
		"a-blog cms",
		"e107",
		"eSyndiCat",
		"eZ Publish",
		"experiencedCMS",
		"gitlist",
		"h5ai",
		"iEXExchanger",
		"iPresta",
		"osCommerce",
		"osTicket",
		"ownCloud",
		"papaya CMS",
		"phpAlbum",
		"phpBB",
		"phpCMS",
		"phpDocumentor",
		"phpMyAdmin",
		"phpPgAdmin",
		"phpRS",
		"phpSQLiteCMS",
		"phpwind",
		"pinoox",
		"punBB",
		"uKnowva",
		"vBulletin",
		"vibecommerce",
		"wpBakery",
		"wpCache",
	}
}

func NodeSoftwareList() []string {
	return []string{
		"AdonisJS",
		"ApostropheCMS",
		"AquilaCMS",
		"Bubble",
		"Catberry.js",
		"Drubbit",
		"Duel",
		"Easy Orders",
		"Etherpad",
		"Express",
		"Fleksa",
		"Front-Commerce",
		"Ghost",
		"Hexo",
		"Instapage",
		"Karma",
		"Kibana",
		"Koa",
		"Marko",
		"Medium",
		"Meteor",
		"MyWebsite Now",
		"Next.js",
		"NodeBB",
		"Nuxt.js",
		"PencilBlue",
		"Phoenix",
		"Retype",
		"Sapper",
		"Socket.io",
		"Storeino",
		"SvelteKit",
		"T1 Paginas",
		"UmiJs",
		"Vnda",
		"Weblium",
		"Wiki.js",
		"Wuilt",
		"actionhero.js",
		"eDokan",
		"enduro.js",
		"total.js",
	}
}
