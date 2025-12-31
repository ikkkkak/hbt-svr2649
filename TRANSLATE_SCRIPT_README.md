# Translation Script for Properties

## Overview

This script (`translate_all_properties.go`) translates all property titles and descriptions to English, French, and Arabic using the MarianMT API, and stores the translations in the database.

## What It Does

1. **Fetches all properties** from the database:
   - Rent properties (`Property` model)
   - Property sales (`PropertySale` model)
   - Landmarks (`Landmark` model)

2. **Uses ORIGINAL (non-translated) titles and descriptions**:
   - Reads `prop.Title` and `prop.Description` (the source, non-translated values)
   - These are the original values stored in the database

3. **Translates to all languages**:
   - English (en)
   - French (fr)
   - Arabic (ar)

4. **Stores translations** in:
   - `title_translations` (JSONB field)
   - `description_translations` (JSONB field)

## How to Run

### Prerequisites

1. Set the `MARIANMT_URL` environment variable (or it will use the default):
   ```bash
   export MARIANMT_URL="https://librerender-1.onrender.com/translate"
   ```

2. Set your database connection string:
   ```bash
   export DB_CONNECTION_STRING="your_database_connection_string"
   ```

### Running the Script

```bash
cd apartmentscloneserver
go run translate_all_properties.go
```

Or compile and run:
```bash
go build -o translate_properties translate_all_properties.go
./translate_properties
```

## What Gets Translated

### For Rent Properties (`Property`):
- ✅ Title → `title_translations`
- ✅ Description → `description_translations`
- ✅ Neighborhood Description → `neighborhood_description_translations`

### For Property Sales (`PropertySale`):
- ✅ Title → `title_translations`
- ✅ Description → `description_translations`

### For Landmarks (`Landmark`):
- ✅ Title → `title_translations`
- ✅ Description → `description_translations`

## Important Notes

1. **Uses Original Values**: The script uses the ORIGINAL titles and descriptions from the database (the `title` and `description` fields), NOT the translated versions.

2. **Skips Existing Translations**: If a property already has complete translations (en, fr, ar), it will skip that property to save API calls.

3. **API Rate Limiting**: The script includes a 200ms delay between API calls to be gentle on the MarianMT API.

4. **Error Handling**: If translation fails, the script logs the error but continues processing other properties.

5. **Validation**: The script validates that translations are different from the original (when appropriate) and logs warnings if translations seem incorrect.

## Output

The script provides detailed logging:
- Progress for each property
- Translation results
- Summary at the end showing:
  - Number of properties updated
  - Number of errors encountered

## Example Output

```
🚀 Starting translation script for all properties...
📋 This script will:
   1. Fetch all properties (rent and sale)
   2. Get their ORIGINAL (non-translated) titles and descriptions
   3. Translate them to en, fr, ar using MarianMT API
   4. Store translations in title_translations and description_translations fields

🧪 Testing MarianMT API connection...
✅ MarianMT test passed! API is working correctly.

📦 Processing Properties (Rent)...
Found 10 properties to process

[1/10] Processing Property ID: 1
  📝 Translating ORIGINAL title: Beautiful apartment in the city
  📊 Translation results:
    ✅ en: Beautiful apartment in the city
    ✅ fr: Bel appartement en ville
    ✅ ar: شقة جميلة في المدينة
  ✅ Updated property 1 with translations

...

📊 TRANSLATION SUMMARY
============================================================
✅ Properties updated: 8
❌ Properties errors: 0
✅ Property Sales updated: 5
❌ Property Sales errors: 0
✅ Landmarks updated: 3
❌ Landmarks errors: 0
============================================================
🎉 Translation script completed!
```

## Troubleshooting

1. **API Connection Issues**: 
   - Check if MarianMT API is accessible
   - Render cold starts may take up to 60 seconds
   - Verify `MARIANMT_URL` is correct

2. **Database Connection**:
   - Ensure `DB_CONNECTION_STRING` is set correctly
   - Check database permissions

3. **Translation Quality**:
   - The script validates translations
   - If translations seem incorrect, check the MarianMT API logs
   - Some translations may be the same as original if source language matches target

