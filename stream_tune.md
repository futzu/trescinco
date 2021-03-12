
### Tuning PAT and PMT packet parsing in threefive.Stream

* step 1: __cProfile'd the Stream class parsing 3.7GB of MPEGTS video for SCTE35.__
 ```sh
 
        106,143,354 function calls (106142526 primitive calls) in 36.231 seconds


    75710    0.025    0.000    0.025    0.000 stream.py:166(_parse_length)
    37840    0.007    0.000    0.007    0.000 stream.py:173(_parse_program_number)
    
 20859289    5.287    0.000    8.727    0.000 stream.py:180(_parser)
 
     8399    0.448    0.000    1.427    0.000 stream.py:247(_program_association_table)
     
    37840    0.325    0.000    0.638    0.000 stream.py:257(_program_map_table)
    37840    0.105    0.000    0.107    0.000 stream.py:296(_parse_program_streams)

```
* Step 2: Added Stream._last_pat (type bytes, holds last pat packet payload) and Stream._last_pmt (type dict, maps pmt_pid and  packet payload)

* Step 3: Added comparison checks in Stream._parser(pkt) to skip parsing for PAT or PMT packets with the samer payload.


* Step 4: __cProfile'd the Stream class parsing 3.7GB of MPEGTS video for SCTE35.__ with the changes.
```sh
       105,227,586 function calls (105226758 primitive calls) in 32.990 seconds

       50    0.000    0.000    0.000    0.000 stream.py:168(_parse_length)
       10    0.000    0.000    0.000    0.000 stream.py:175(_parse_program_number)
       
 20859289    5.141    0.000    6.502    0.000 stream.py:182(_parser
 
        1    0.000    0.000    0.001    0.001 stream.py:258(_program_association_table)
        
       10    0.000    0.000    0.002    0.000 stream.py:268(_program_map_table)
       10    0.000    0.000    0.002    0.000 stream.py:307(_parse_program_streams)


```