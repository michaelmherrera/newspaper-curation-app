# Development Notes

Completed the two following tasks. Now need to verify that it works with open-oni

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

## Errata

- 1961-12-01 AKA 2_06
  - Only has one page
- Check other errata in NCA
- 4_08 skipped (1964-01-24)
- 6_01 (labelled in ocr as 7_01) may have been skipped due to labelling error

- In the 1966-1967 school year, Aristocrat was in their 7th volume, but erroneously labelled it their 6th volume. They noticed the issue the following year and skipped to the 8th volume for the 1967-1978 year

- Much of 8th edition didn't have volume, edition or date labelling. Had to interpelate. Same for vol 9


Volume 1 ends 1961
V2: 1961-1962
V3: 1962-1963
V4: 1963-1964
V5: 1964-1965
V6: 1965-1966
V7: 1966-1967
V8: 1967-1968
V9: 1968-1969

## Missing issues

1961-03-15 1961-03-15
1961-04-05 1961-04-05
1961-12-01 1961-12-01
1962-02-16 1962-02-16
1962-03-02 1962-03-02
1962-03-16 1962-03-16
1964-04-24 1964-04-24
1965-06-17 1965-06-17
1966-04-22 1966-04-22
1966-05-20 1966-05-20
1984-06-08 1984-06-08 
1985-10-28 1985-10-28
1985-12-03 1985-12-03
1985-12-20 1985-12-20
1986-03-28 1986-03-28
1986-04-23 1986-04-23
1986-06-06 1986-06-06
1986-09-26 1986-09-26
1986-10-24 1986-10-24
1986-11-19 1986-11-19
1986-12-18 1986-12-18
1987-02-17 1987-02-17
1987-03-20 1987-03-20
1987-04-24 1987-04-24
1987-06-05 1987-06-05
1987-09-28 1987-09-28
1987-10-23 1987-10-23
1987-11-20 1987-11-20
1987-12-18 1987-12-18
1988-03-25 1988-03-25
1988-04-22 1988-04-22
1988-06-03 1988-06-03