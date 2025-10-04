-- Create categories table
CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    type VARCHAR(20) NOT NULL CHECK (type IN ('property', 'experience')),
    name JSONB NOT NULL,
    icon VARCHAR(50) NOT NULL,
    description JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create amenities table
CREATE TABLE IF NOT EXISTS amenities (
    id SERIAL PRIMARY KEY,
    name JSONB NOT NULL,
    icon VARCHAR(50) NOT NULL,
    category VARCHAR(50) NOT NULL,
    description JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create property_categories junction table
CREATE TABLE IF NOT EXISTS property_categories (
    property_id INTEGER REFERENCES properties(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (property_id, category_id)
);

-- Create property_amenities junction table
CREATE TABLE IF NOT EXISTS property_amenities (
    property_id INTEGER REFERENCES properties(id) ON DELETE CASCADE,
    amenity_id INTEGER REFERENCES amenities(id) ON DELETE CASCADE,
    PRIMARY KEY (property_id, amenity_id)
);

-- Create experience_categories junction table
CREATE TABLE IF NOT EXISTS experience_categories (
    experience_id INTEGER REFERENCES experiences(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (experience_id, category_id)
);

-- Add indexes for better performance
CREATE INDEX IF NOT EXISTS idx_categories_type ON categories(type);
CREATE INDEX IF NOT EXISTS idx_categories_active ON categories(is_active);
CREATE INDEX IF NOT EXISTS idx_amenities_category ON amenities(category);
CREATE INDEX IF NOT EXISTS idx_amenities_active ON amenities(is_active);
CREATE INDEX IF NOT EXISTS idx_property_categories_property_id ON property_categories(property_id);
CREATE INDEX IF NOT EXISTS idx_property_categories_category_id ON property_categories(category_id);
CREATE INDEX IF NOT EXISTS idx_property_amenities_property_id ON property_amenities(property_id);
CREATE INDEX IF NOT EXISTS idx_property_amenities_amenity_id ON property_amenities(amenity_id);
CREATE INDEX IF NOT EXISTS idx_experience_categories_experience_id ON experience_categories(experience_id);
CREATE INDEX IF NOT EXISTS idx_experience_categories_category_id ON experience_categories(category_id);

-- Insert property categories
INSERT INTO categories (type, name, icon, description, sort_order) VALUES
('property', '{"en": "Apartment", "fr": "Appartement", "ar": "شقة"}', 'Buildings', '{"en": "Modern apartments in Nouakchott and other cities", "fr": "Appartements modernes à Nouakchott et autres villes", "ar": "شقق حديثة في نواكشوط ومدن أخرى"}', 1),
('property', '{"en": "House", "fr": "Maison", "ar": "منزل"}', 'House', '{"en": "Traditional and modern houses", "fr": "Maisons traditionnelles et modernes", "ar": "منازل تقليدية وحديثة"}', 2),
('property', '{"en": "Villa", "fr": "Villa", "ar": "فيلا"}', 'HouseLine', '{"en": "Luxury villas with gardens and pools", "fr": "Villas de luxe avec jardins et piscines", "ar": "فيلات فاخرة مع حدائق ومسابح"}', 3),
('property', '{"en": "Riyad", "fr": "Riyad", "ar": "رياض"}', 'Tree', '{"en": "Traditional Mauritanian courtyard houses", "fr": "Maisons traditionnelles mauritaniennes avec cour", "ar": "منازل تقليدية موريتانية مع فناء"}', 4),
('property', '{"en": "Guest House", "fr": "Maison d''hôtes", "ar": "بيت ضيافة"}', 'Users', '{"en": "Traditional guest houses and family homes", "fr": "Maisons d''hôtes traditionnelles et maisons familiales", "ar": "بيوت ضيافة تقليدية ومنازل عائلية"}', 5),
('property', '{"en": "Hotel", "fr": "Hôtel", "ar": "فندق"}', 'Buildings', '{"en": "Hotels and business accommodations", "fr": "Hôtels et hébergements d''affaires", "ar": "فنادق وإقامات تجارية"}', 6),
('property', '{"en": "Beach House", "fr": "Maison de plage", "ar": "منزل شاطئي"}', 'Waves', '{"en": "Beachfront properties in Nouadhibou and coastal areas", "fr": "Propriétés en bord de mer à Nouadhibou et zones côtières", "ar": "عقارات على الشاطئ في نواذيبو والمناطق الساحلية"}', 7),
('property', '{"en": "Desert Camp", "fr": "Camp du désert", "ar": "مخيم صحراوي"}', 'Tent', '{"en": "Traditional desert camps and nomadic accommodations", "fr": "Camps du désert traditionnels et hébergements nomades", "ar": "مخيمات صحراوية تقليدية وإقامات بدوية"}', 8),
('property', '{"en": "Business Space", "fr": "Espace d''affaires", "ar": "مساحة تجارية"}', 'Briefcase', '{"en": "Office spaces and business accommodations", "fr": "Espaces de bureau et hébergements d''affaires", "ar": "مساحات مكتبية وإقامات تجارية"}', 9),
('property', '{"en": "Student Housing", "fr": "Logement étudiant", "ar": "سكن طلابي"}', 'GraduationCap', '{"en": "Student accommodations near universities", "fr": "Hébergements étudiants près des universités", "ar": "إقامات طلابية قرب الجامعات"}', 10);

-- Insert experience categories
INSERT INTO categories (type, name, icon, description, sort_order) VALUES
('experience', '{"en": "Cultural Tour", "fr": "Tour culturel", "ar": "جولة ثقافية"}', 'MapPin', '{"en": "Explore Mauritanian culture and heritage", "fr": "Explorez la culture et le patrimoine mauritanien", "ar": "استكشف الثقافة والتراث الموريتاني"}', 1),
('experience', '{"en": "Desert Safari", "fr": "Safari dans le désert", "ar": "رحلة سفاري صحراوية"}', 'Car', '{"en": "Adventure tours in the Sahara Desert", "fr": "Tours d''aventure dans le désert du Sahara", "ar": "رحلات مغامرة في الصحراء الكبرى"}', 2),
('experience', '{"en": "Camel Riding", "fr": "Balade à dos de chameau", "ar": "ركوب الجمال"}', 'Horse', '{"en": "Traditional camel riding experiences", "fr": "Expériences traditionnelles de balade à dos de chameau", "ar": "تجارب تقليدية لركوب الجمال"}', 3),
('experience', '{"en": "Fishing Trip", "fr": "Sortie de pêche", "ar": "رحلة صيد"}', 'Fish', '{"en": "Deep sea and coastal fishing experiences", "fr": "Expériences de pêche en haute mer et côtière", "ar": "تجارب صيد في أعماق البحر والساحل"}', 4),
('experience', '{"en": "Cooking Class", "fr": "Cours de cuisine", "ar": "فصل طبخ"}', 'ChefHat', '{"en": "Learn traditional Mauritanian cuisine", "fr": "Apprenez la cuisine traditionnelle mauritanienne", "ar": "تعلم المطبخ الموريتاني التقليدي"}', 5),
('experience', '{"en": "Music Performance", "fr": "Spectacle musical", "ar": "عرض موسيقي"}', 'MusicNote', '{"en": "Traditional Mauritanian music and performances", "fr": "Musique et spectacles traditionnels mauritaniens", "ar": "موسيقى وعروض موريتانية تقليدية"}', 6),
('experience', '{"en": "Handicraft Workshop", "fr": "Atelier d''artisanat", "ar": "ورشة حرف يدوية"}', 'Hammer', '{"en": "Learn traditional Mauritanian crafts", "fr": "Apprenez l''artisanat traditionnel mauritanien", "ar": "تعلم الحرف اليدوية الموريتانية التقليدية"}', 7),
('experience', '{"en": "City Tour", "fr": "Visite de la ville", "ar": "جولة في المدينة"}', 'Buildings', '{"en": "Guided tours of Nouakchott and other cities", "fr": "Visites guidées de Nouakchott et autres villes", "ar": "جولات إرشادية في نواكشوط ومدن أخرى"}', 8),
('experience', '{"en": "Beach Activity", "fr": "Activité de plage", "ar": "نشاط شاطئي"}', 'Waves', '{"en": "Beach activities and water sports", "fr": "Activités de plage et sports nautiques", "ar": "أنشطة شاطئية ورياضات مائية"}', 9),
('experience', '{"en": "Stargazing", "fr": "Observation des étoiles", "ar": "مراقبة النجوم"}', 'Star', '{"en": "Desert stargazing and astronomy experiences", "fr": "Observation des étoiles dans le désert et expériences d''astronomie", "ar": "مراقبة النجوم في الصحراء وتجارب الفلك"}', 10);

-- Insert amenities
INSERT INTO amenities (name, icon, category, description, sort_order) VALUES
-- Essential Amenities
('{"en": "WiFi", "fr": "WiFi", "ar": "واي فاي"}', 'WifiHigh', 'essential', '{"en": "High-speed internet connection", "fr": "Connexion internet haut débit", "ar": "اتصال إنترنت عالي السرعة"}', 1),
('{"en": "Air Conditioning", "fr": "Climatisation", "ar": "تكييف هواء"}', 'Snowflake', 'essential', '{"en": "Air conditioning for hot weather", "fr": "Climatisation pour temps chaud", "ar": "تكييف هواء للطقس الحار"}', 2),
('{"en": "Heating", "fr": "Chauffage", "ar": "تدفئة"}', 'Thermometer', 'essential', '{"en": "Heating system for cooler months", "fr": "Système de chauffage pour mois plus frais", "ar": "نظام تدفئة للأشهر الباردة"}', 3),
('{"en": "Free Parking", "fr": "Parking gratuit", "ar": "موقف سيارات مجاني"}', 'Car', 'essential', '{"en": "Free parking space available", "fr": "Place de parking gratuite disponible", "ar": "مكان وقوف سيارات مجاني متاح"}', 4),

-- Safety Amenities
('{"en": "Smoke Detector", "fr": "Détecteur de fumée", "ar": "كاشف الدخان"}', 'Warning', 'safety', '{"en": "Smoke detection system", "fr": "Système de détection de fumée", "ar": "نظام كشف الدخان"}', 5),
('{"en": "First Aid Kit", "fr": "Trousse de premiers secours", "ar": "حقيبة إسعافات أولية"}', 'FirstAid', 'safety', '{"en": "First aid supplies available", "fr": "Fournitures de premiers secours disponibles", "ar": "مستلزمات الإسعافات الأولية متاحة"}', 6),
('{"en": "Security Cameras", "fr": "Caméras de sécurité", "ar": "كاميرات أمنية"}', 'Camera', 'safety', '{"en": "Security camera system", "fr": "Système de caméras de sécurité", "ar": "نظام كاميرات أمنية"}', 7),
('{"en": "Secure Compound", "fr": "Compound sécurisé", "ar": "مجمع آمن"}', 'Shield', 'safety', '{"en": "Gated and secured residential compound", "fr": "Compound résidentiel fermé et sécurisé", "ar": "مجمع سكني محاط ببوابات وآمن"}', 8),

-- Kitchen Amenities
('{"en": "Kitchen", "fr": "Cuisine", "ar": "مطبخ"}', 'CookingPot', 'kitchen', '{"en": "Fully equipped kitchen", "fr": "Cuisine entièrement équipée", "ar": "مطبخ مجهز بالكامل"}', 9),
('{"en": "Refrigerator", "fr": "Réfrigérateur", "ar": "ثلاجة"}', 'Refrigerator', 'kitchen', '{"en": "Refrigerator for food storage", "fr": "Réfrigérateur pour stockage alimentaire", "ar": "ثلاجة لتخزين الطعام"}', 10),
('{"en": "Microwave", "fr": "Micro-ondes", "ar": "ميكروويف"}', 'Microwave', 'kitchen', '{"en": "Microwave oven available", "fr": "Four à micro-ondes disponible", "ar": "فرن ميكروويف متاح"}', 11),
('{"en": "Coffee Maker", "fr": "Machine à café", "ar": "آلة قهوة"}', 'Coffee', 'kitchen', '{"en": "Coffee brewing equipment", "fr": "Équipement de préparation de café", "ar": "معدات تحضير القهوة"}', 12),

-- Bathroom Amenities
('{"en": "Hot Water", "fr": "Eau chaude", "ar": "ماء ساخن"}', 'Drop', 'bathroom', '{"en": "Hot water available 24/7", "fr": "Eau chaude disponible 24h/24", "ar": "ماء ساخن متاح 24/7"}', 13),
('{"en": "Bathtub", "fr": "Baignoire", "ar": "حوض استحمام"}', 'Bathtub', 'bathroom', '{"en": "Bathtub for relaxation", "fr": "Baignoire pour relaxation", "ar": "حوض استحمام للاسترخاء"}', 14),
('{"en": "Hair Dryer", "fr": "Sèche-cheveux", "ar": "مجفف شعر"}', 'Wind', 'bathroom', '{"en": "Hair drying equipment", "fr": "Équipement de séchage de cheveux", "ar": "معدات تجفيف الشعر"}', 15),

-- Bedroom Amenities
('{"en": "Bed Linens", "fr": "Draps de lit", "ar": "ملاءات السرير"}', 'Bed', 'bedroom', '{"en": "Clean bed linens provided", "fr": "Draps de lit propres fournis", "ar": "ملاءات سرير نظيفة متوفرة"}', 16),
('{"en": "Wardrobe", "fr": "Garde-robe", "ar": "خزانة ملابس"}', 'Shirt', 'bedroom', '{"en": "Clothing storage space", "fr": "Espace de rangement pour vêtements", "ar": "مساحة تخزين الملابس"}', 17),
('{"en": "Desk", "fr": "Bureau", "ar": "مكتب"}', 'Notebook', 'bedroom', '{"en": "Work desk available", "fr": "Bureau de travail disponible", "ar": "مكتب عمل متاح"}', 18),

-- Outdoor Amenities
('{"en": "Balcony", "fr": "Balcon", "ar": "شرفة"}', 'Balcony', 'outdoor', '{"en": "Private balcony with view", "fr": "Balcon privé avec vue", "ar": "شرفة خاصة مع إطلالة"}', 19),
('{"en": "Garden", "fr": "Jardin", "ar": "حديقة"}', 'Tree', 'outdoor', '{"en": "Private garden space", "fr": "Espace jardin privé", "ar": "مساحة حديقة خاصة"}', 20),
('{"en": "Swimming Pool", "fr": "Piscine", "ar": "مسبح"}', 'SwimmingPool', 'outdoor', '{"en": "Swimming pool access", "fr": "Accès à la piscine", "ar": "وصول إلى المسبح"}', 21),
('{"en": "BBQ Area", "fr": "Zone BBQ", "ar": "منطقة شواء"}', 'Fire', 'outdoor', '{"en": "Barbecue and outdoor cooking area", "fr": "Zone barbecue et cuisine extérieure", "ar": "منطقة شواء وطبخ خارجي"}', 22),

-- Entertainment Amenities
('{"en": "TV", "fr": "Télévision", "ar": "تلفزيون"}', 'Television', 'entertainment', '{"en": "Television with cable/satellite", "fr": "Télévision avec câble/satellite", "ar": "تلفزيون مع كابل/ساتل"}', 23),
('{"en": "Sound System", "fr": "Système audio", "ar": "نظام صوتي"}', 'SpeakerHigh', 'entertainment', '{"en": "Audio system for music", "fr": "Système audio pour musique", "ar": "نظام صوتي للموسيقى"}', 24),
('{"en": "Board Games", "fr": "Jeux de société", "ar": "ألعاب الطاولة"}', 'GameController', 'entertainment', '{"en": "Board games and entertainment", "fr": "Jeux de société et divertissement", "ar": "ألعاب الطاولة والترفيه"}', 25),

-- Mauritania-Specific Amenities
('{"en": "Generator", "fr": "Générateur", "ar": "مولد كهرباء"}', 'Lightning', 'mauritania_specific', '{"en": "Backup generator for power outages", "fr": "Générateur de secours pour pannes d''électricité", "ar": "مولد احتياطي لانقطاع الكهرباء"}', 26),
('{"en": "Water Tank", "fr": "Réservoir d''eau", "ar": "خزان ماء"}', 'Drop', 'mauritania_specific', '{"en": "Water storage tank for reliable supply", "fr": "Réservoir de stockage d''eau pour approvisionnement fiable", "ar": "خزان تخزين ماء لإمداد موثوق"}', 27),
('{"en": "Mosque Nearby", "fr": "Mosquée à proximité", "ar": "مسجد قريب"}', 'Mosque', 'mauritania_specific', '{"en": "Mosque within walking distance", "fr": "Mosquée à distance de marche", "ar": "مسجد على مسافة قريبة"}', 28),
('{"en": "Halal Food Available", "fr": "Nourriture halal disponible", "ar": "طعام حلال متاح"}', 'ForkKnife', 'mauritania_specific', '{"en": "Halal food options nearby", "fr": "Options de nourriture halal à proximité", "ar": "خيارات طعام حلال قريبة"}', 29),
('{"en": "Desert View", "fr": "Vue sur le désert", "ar": "إطلالة صحراوية"}', 'Mountains', 'mauritania_specific', '{"en": "Beautiful desert landscape view", "fr": "Belle vue sur le paysage désertique", "ar": "إطلالة جميلة على المشهد الصحراوي"}', 30),
('{"en": "Traditional Furniture", "fr": "Mobilier traditionnel", "ar": "أثاث تقليدي"}', 'Armchair', 'mauritania_specific', '{"en": "Traditional Mauritanian furniture and decor", "fr": "Mobilier et décoration traditionnels mauritaniens", "ar": "أثاث وديكور موريتاني تقليدي"}', 31);
