# Property Translation Script

This script translates all property titles, descriptions, and neighborhood descriptions for:
- **Properties** (rent properties)
- **Property Sales**
- **Landmarks**

## Prerequisites

1. **LibreTranslate Server**: Make sure LibreTranslate is running
   - Default URL: `https://librerender.onrender.com/translate`
   - Or set `LIBRETRANSLATE_URL` environment variable

2. **Database Connection**: The script uses the same database connection as the main server
   - Requires `DB_CONNECTION_STRING` environment variable
   - Or `.env` file with `DB_CONNECTION_STRING`

## How to Run

### Option 1: Using Go Run
```bash
cd apartmentscloneserver
go run translate_all_properties.go
```

### Option 2: Build and Run
```bash
cd apartmentscloneserver
go build -o translate_properties translate_all_properties.go
./translate_properties
```

## Environment Variables

```bash
# Required
DB_CONNECTION_STRING=postgres://user:password@localhost:5432/dbname

# Optional (defaults to https://librerender.onrender.com/translate)
LIBRETRANSLATE_URL=http://192.168.100.51:5000/translate
```

## What It Does

1. **Fetches all records** from:
   - `properties` table
   - `property_sales` table
   - `landmarks` table

2. **Checks each record** for missing translations:
   - If `title_translations` is empty or incomplete → translates title
   - If `description_translations` is empty or incomplete → translates description
   - If `neighborhood_description_translations` is empty or incomplete → translates neighborhood description

3. **Translates to 3 languages**:
   - English (`en`)
   - French (`fr`)
   - Arabic (`ar`)

4. **Updates the database** with the translations stored as JSONB:
   ```json
   {
     "en": "English text",
     "fr": "French text",
     "ar": "Arabic text"
   }
   ```

## Features

- ✅ **Smart Detection**: Only translates if translations are missing or incomplete
- ✅ **Rate Limiting**: 200ms delay between translations to be gentle with the API
- ✅ **Error Handling**: Continues processing even if individual records fail
- ✅ **Progress Logging**: Shows detailed progress for each record
- ✅ **Summary Report**: Shows total updated records and errors at the end

## Example Output

```
🚀 Starting translation script for all properties...
✅ Using LibreTranslate URL: http://192.168.100.51:5000/translate

📦 Processing Properties (Rent)...
Found 150 properties to process

[1/150] Processing Property ID: 1
  📝 Translating title: Beautiful Apartment in Nouakchott
  📝 Translating description...
  ✅ Updated property 1 with translations

[2/150] Processing Property ID: 2
  ⏭️  Property 2 already has all translations

...

============================================================
📊 TRANSLATION SUMMARY
============================================================
✅ Properties updated: 120
❌ Properties errors: 0
✅ Property Sales updated: 45
❌ Property Sales errors: 0
✅ Landmarks updated: 30
❌ Landmarks errors: 0
============================================================
🎉 Translation script completed!
```

## Notes

- The script uses the existing `services.TranslateAllLanguages()` function
- Translations are stored as JSONB in the database
- The script is idempotent - safe to run multiple times
- It will only translate missing or incomplete translations

