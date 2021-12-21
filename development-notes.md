# Development Notes

1. There is an issue with the resolution of the embedded images. 
    - Supress issue and deploy to ONI, to see if it causes issue with deployment

2. Alter quality in `settings-example` because getting grainy visuals.
    - Currently, QUALITY=62.5


## ONI

1. Add the following to `core/fixtures/awardees.json` before initial run so that it's added to the db
```json
{
    "pk": "aol",
    "model": "core.Awardee",
    "fields": {
      "name": "Aragon Outlook, Aragon High School; San Mateo, CA",
      "created": "2009-02-09T06:44:45+00:00"
    }
  },
```
1. Set the following in `onisite/settings_local_example.py`

```python
MARC_RETRIEVAL_URLFORMAT = 'https://raw.githubusercontent.com/michaelmherrera/marcs/main/%s/marc.xml'
```

1. Add to `countries.json`:

```
{
    "pk": "aol", 
    "model": "core.country", 
    "fields": {
      "region": "North America", 
      "name": "California"
    }
  }, 
```

1. Load the marc file manually
```bash
./manage.py load_titles /opt/openoni/data/marc.xml

```

1. Replace all occurances of `000000000` with `sn000000000` (including `data/000000000`)

## Notes

- `process_ocr` in batch_loading is passed page object. The page object is supposed to contain the attribute solr_doc (page.solr_doc)
- `WARNING:core.batch_loader:unable to find reel number in page metadata`

`core/title_loader.py` loads an end date
End date required. check `models.py in func solr_doc for class Title`
